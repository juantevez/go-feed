package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/notification-service/internal/application/port/input"
	"github.com/juantevez/my-ig/notification-service/internal/domain/notification"
)

// ── fakes ─────────────────────────────────────────────────────────────────────

type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type pair struct{ follower, following uuid.UUID }

type fakeFollows struct{ stateAt map[pair]time.Time }

func (f *fakeFollows) apply(followerID, followingID uuid.UUID, at time.Time) bool {
	k := pair{followerID, followingID}
	if prev, ok := f.stateAt[k]; ok && !prev.Before(at) {
		return false
	}
	f.stateAt[k] = at
	return true
}
func (f *fakeFollows) ApplyFollow(_ context.Context, a, b uuid.UUID, at time.Time) (bool, error) {
	return f.apply(a, b, at), nil
}
func (f *fakeFollows) ApplyUnfollow(_ context.Context, a, b uuid.UUID, at time.Time) (bool, error) {
	return f.apply(a, b, at), nil
}

type fakeRepo struct {
	saved      []*notification.Notification
	lastOffset int
}

func (r *fakeRepo) Save(_ context.Context, n *notification.Notification) (bool, error) {
	r.saved = append(r.saved, n)
	return true, nil
}
func (r *fakeRepo) GetByUser(_ context.Context, _ uuid.UUID, _, offset int, _ bool) ([]*notification.Notification, error) {
	r.lastOffset = offset
	return nil, nil
}
func (r *fakeRepo) MarkRead(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (r *fakeRepo) MarkAllRead(context.Context, uuid.UUID) error         { return nil }
func (r *fakeRepo) CountUnread(context.Context, uuid.UUID) (int, error)  { return 0, nil }
func (r *fakeRepo) DeleteNewFollower(_ context.Context, target, follower uuid.UUID) (int64, error) {
	kept := r.saved[:0]
	var n int64
	for _, s := range r.saved {
		if s.TargetUserID == target && s.SourceUserID == follower && s.Type == notification.TypeNewFollower {
			n++
			continue
		}
		kept = append(kept, s)
	}
	r.saved = kept
	return n, nil
}

type fakePusher struct{ pushed chan uuid.UUID }

func (p *fakePusher) Push(_ context.Context, _ uuid.UUID, n *notification.Notification) error {
	p.pushed <- n.ID
	return nil
}

func newService() (*NotificationService, *fakeRepo, *fakePusher) {
	repo := &fakeRepo{}
	pusher := &fakePusher{pushed: make(chan uuid.UUID, 10)}
	return NewNotificationService(fakeTx{}, repo, &fakeFollows{stateAt: map[pair]time.Time{}}, pusher), repo, pusher
}

func newFollower(t *testing.T, target, follower uuid.UUID) *notification.Notification {
	t.Helper()
	n, err := notification.New(target, follower, notification.TypeNewFollower, map[string]string{"follower_id": follower.String()})
	if err != nil {
		t.Fatal(err)
	}
	return n.FromEvent(uuid.New())
}

func pushes(p *fakePusher) int {
	time.Sleep(50 * time.Millisecond) // el push corre en una goroutine
	return len(p.pushed)
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestFollowUnfollowFollowLeavesOneNotification(t *testing.T) {
	svc, repo, pusher := newService()
	ctx, target, follower := context.Background(), uuid.New(), uuid.New()
	t0 := time.Now()

	if err := svc.NotifyNewFollower(ctx, newFollower(t, target, follower), t0); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveNewFollower(ctx, follower, target, t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(repo.saved) != 0 {
		t.Fatalf("after unfollow: %d notifications, want 0", len(repo.saved))
	}
	if err := svc.NotifyNewFollower(ctx, newFollower(t, target, follower), t0.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(repo.saved) != 1 {
		t.Errorf("after re-follow: %d notifications, want 1", len(repo.saved))
	}
	if got := pushes(pusher); got != 2 {
		t.Errorf("pushes = %d, want 2 (one per follow)", got)
	}
}

// El unfollow se procesa antes que su follow (subscriptions separadas): el follow
// tardío no debe crear la notificación.
func TestUnfollowBeforeItsFollow(t *testing.T) {
	svc, repo, pusher := newService()
	ctx, target, follower := context.Background(), uuid.New(), uuid.New()
	t0 := time.Now()

	if err := svc.RemoveNewFollower(ctx, follower, target, t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := svc.NotifyNewFollower(ctx, newFollower(t, target, follower), t0); err != nil {
		t.Fatal(err)
	}
	if len(repo.saved) != 0 {
		t.Errorf("%d notifications, want 0: the follow is older than the unfollow", len(repo.saved))
	}
	if got := pushes(pusher); got != 0 {
		t.Errorf("pushes = %d, want 0", got)
	}
}

// Una reentrega del mismo follow no notifica dos veces.
func TestRedeliveredFollowIsIgnored(t *testing.T) {
	svc, repo, pusher := newService()
	ctx, target, follower := context.Background(), uuid.New(), uuid.New()
	n, at := newFollower(t, target, follower), time.Now()

	for i := 0; i < 2; i++ {
		if err := svc.NotifyNewFollower(ctx, n, at); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.saved) != 1 {
		t.Errorf("%d notifications, want 1", len(repo.saved))
	}
	if got := pushes(pusher); got != 1 {
		t.Errorf("pushes = %d, want 1", got)
	}
}

func TestNegativeOffsetIsClampedToZero(t *testing.T) {
	svc, repo, _ := newService()
	if _, err := svc.GetNotifications(context.Background(), input.GetNotificationsQuery{UserID: uuid.New(), Offset: -5}); err != nil {
		t.Fatal(err)
	}
	if repo.lastOffset != 0 {
		t.Errorf("offset passed to repo = %d, want 0", repo.lastOffset)
	}
}
