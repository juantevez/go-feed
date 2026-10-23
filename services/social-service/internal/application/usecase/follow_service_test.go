package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/social-service/internal/application/port/input"
	"github.com/juantevez/my-ig/social-service/internal/domain/follow"
)

// ── fakes ─────────────────────────────────────────────────────────────────────

type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type pair struct{ follower, following uuid.UUID }

// fakeFollows guarda las relaciones con su flag mutual, como la tabla follows.
type fakeFollows struct {
	rows   map[pair]bool
	locked []pair
}

func (f *fakeFollows) LockPair(_ context.Context, a, b uuid.UUID) error {
	f.locked = append(f.locked, pair{a, b})
	return nil
}
func (f *fakeFollows) Save(_ context.Context, fl *follow.Follow) error {
	k := pair{fl.FollowerID, fl.FollowingID}
	if _, ok := f.rows[k]; ok {
		return follow.ErrAlreadyFollowing
	}
	f.rows[k] = fl.Mutual
	return nil
}
func (f *fakeFollows) Delete(_ context.Context, a, b uuid.UUID) error {
	if _, ok := f.rows[pair{a, b}]; !ok {
		return follow.ErrNotFollowing
	}
	delete(f.rows, pair{a, b})
	return nil
}
func (f *fakeFollows) Exists(_ context.Context, a, b uuid.UUID) (bool, error) {
	_, ok := f.rows[pair{a, b}]
	return ok, nil
}

// UpdateMutual con la misma semántica que el SQL: las dos filas del par.
func (f *fakeFollows) UpdateMutual(_ context.Context, a, b uuid.UUID, mutual bool) error {
	for _, k := range []pair{{a, b}, {b, a}} {
		if _, ok := f.rows[k]; ok {
			f.rows[k] = mutual
		}
	}
	return nil
}
func (f *fakeFollows) GetFollowers(context.Context, uuid.UUID, int, int) ([]*follow.Follow, error) {
	return nil, nil
}
func (f *fakeFollows) GetFollowing(context.Context, uuid.UUID, int, int) ([]*follow.Follow, error) {
	return nil, nil
}
func (f *fakeFollows) GetStats(context.Context, uuid.UUID) (*follow.Stats, error) { return nil, nil }

type fakeUsers struct{ known map[uuid.UUID]bool }

func (u *fakeUsers) Save(_ context.Context, id uuid.UUID, _ string, _ time.Time) error {
	u.known[id] = true
	return nil
}
func (u *fakeUsers) Exists(_ context.Context, id uuid.UUID) (bool, error) { return u.known[id], nil }

type fakePublisher struct{ topics []string }

func (p *fakePublisher) Publish(_ context.Context, topic string, _ any) error {
	p.topics = append(p.topics, topic)
	return nil
}

func newService(users ...uuid.UUID) (*FollowService, *fakeFollows, *fakePublisher) {
	follows := &fakeFollows{rows: map[pair]bool{}}
	known := &fakeUsers{known: map[uuid.UUID]bool{}}
	for _, u := range users {
		known.known[u] = true
	}
	pub := &fakePublisher{}
	return NewFollowService(fakeTx{}, follows, known, pub), follows, pub
}

func followCmd(a, b uuid.UUID) input.FollowCommand {
	return input.FollowCommand{FollowerID: a, FollowingID: b}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestMutualFollowMarksBothRows(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	svc, follows, _ := newService(a, b)
	ctx := context.Background()

	if err := svc.Follow(ctx, followCmd(a, b)); err != nil {
		t.Fatal(err)
	}
	if err := svc.Follow(ctx, followCmd(b, a)); err != nil {
		t.Fatal(err)
	}
	if !follows.rows[pair{a, b}] || !follows.rows[pair{b, a}] {
		t.Errorf("mutual A→B = %v, B→A = %v; want both true", follows.rows[pair{a, b}], follows.rows[pair{b, a}])
	}

	// Al dejar de seguir, el otro lado deja de ser mutuo.
	if err := svc.Unfollow(ctx, input.UnfollowCommand{FollowerID: b, FollowingID: a}); err != nil {
		t.Fatal(err)
	}
	if follows.rows[pair{a, b}] {
		t.Error("A→B still mutual after B unfollowed A")
	}
}

func TestFollowAndUnfollowLockThePair(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	svc, follows, _ := newService(a, b)
	ctx := context.Background()

	_ = svc.Follow(ctx, followCmd(a, b))
	_ = svc.Unfollow(ctx, input.UnfollowCommand{FollowerID: a, FollowingID: b})
	if len(follows.locked) != 2 {
		t.Errorf("LockPair calls = %d, want 2 (follow and unfollow)", len(follows.locked))
	}
}

func TestFollowUnknownUserIsRejected(t *testing.T) {
	a := uuid.New()
	svc, follows, pub := newService(a) // el seguido no está en el directorio

	err := svc.Follow(context.Background(), followCmd(a, uuid.New()))
	if !errors.Is(err, follow.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
	if len(follows.rows) != 0 || len(pub.topics) != 0 {
		t.Errorf("rows = %d, events = %d; want nothing saved or published", len(follows.rows), len(pub.topics))
	}
}

func TestSelfFollowIsRejected(t *testing.T) {
	a := uuid.New()
	svc, _, _ := newService(a)
	if err := svc.Follow(context.Background(), followCmd(a, a)); !errors.Is(err, follow.ErrSelfFollow) {
		t.Fatalf("err = %v, want ErrSelfFollow", err)
	}
}
