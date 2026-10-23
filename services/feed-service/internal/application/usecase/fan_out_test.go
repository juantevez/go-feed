package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/feed-service/internal/application/port/input"
	"github.com/juantevez/my-ig/feed-service/internal/domain/feed"
)

// ── fakes ─────────────────────────────────────────────────────────────────────

type fakeTx struct{ calls int }

func (t *fakeTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	t.calls++
	return fn(ctx)
}

type fakeFeeds struct {
	deleted     map[uuid.UUID]bool
	locked      []uuid.UUID
	inserted    []*feed.FeedEntry
	authorPosts []uuid.UUID
}

func (f *fakeFeeds) LockPost(_ context.Context, postID uuid.UUID) error {
	f.locked = append(f.locked, postID)
	return nil
}
func (f *fakeFeeds) MarkPostDeleted(_ context.Context, postID uuid.UUID, _ time.Time) error {
	f.deleted[postID] = true
	return nil
}
func (f *fakeFeeds) IsPostDeleted(_ context.Context, postID uuid.UUID) (bool, error) {
	return f.deleted[postID], nil
}
func (f *fakeFeeds) SaveAuthorPost(_ context.Context, postID, _ uuid.UUID, _ time.Time) error {
	f.authorPosts = append(f.authorPosts, postID)
	return nil
}
func (f *fakeFeeds) BulkInsert(_ context.Context, entries []*feed.FeedEntry) error {
	f.inserted = append(f.inserted, entries...)
	return nil
}
func (f *fakeFeeds) GetPage(context.Context, uuid.UUID, *feed.Cursor, int) ([]*feed.FeedEntry, error) {
	return nil, nil
}
func (f *fakeFeeds) DeleteByPostID(context.Context, uuid.UUID) error                    { return nil }
func (f *fakeFeeds) DeleteByAuthorFromUser(context.Context, uuid.UUID, uuid.UUID) error { return nil }

type fakeFollowers struct{ ids []uuid.UUID }

func (f *fakeFollowers) GetActiveFollowers(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return f.ids, nil
}

func newFanOut(followers ...uuid.UUID) (*FanOutService, *fakeTx, *fakeFeeds) {
	tx := &fakeTx{}
	feeds := &fakeFeeds{deleted: map[uuid.UUID]bool{}}
	return NewFanOutService(tx, feeds, &fakeFollowers{ids: followers}), tx, feeds
}

func recipients(entries []*feed.FeedEntry) map[uuid.UUID]bool {
	m := map[uuid.UUID]bool{}
	for _, e := range entries {
		m[e.UserID] = true
	}
	return m
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestFanOutPublicReachesAuthorAndFollowers(t *testing.T) {
	f1, f2 := uuid.New(), uuid.New()
	svc, tx, feeds := newFanOut(f1, f2)
	cmd := input.FanOutCommand{PostID: uuid.New(), AuthorID: uuid.New(), PublishedAt: time.Now(), Visibility: "public"}

	if err := svc.FanOut(context.Background(), cmd); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	got := recipients(feeds.inserted)
	if len(got) != 3 || !got[cmd.AuthorID] || !got[f1] || !got[f2] {
		t.Errorf("recipients = %v, want author + 2 followers", got)
	}
	if len(feeds.authorPosts) != 1 {
		t.Errorf("author_posts saves = %d, want 1", len(feeds.authorPosts))
	}
	if tx.calls != 1 || len(feeds.locked) != 1 || feeds.locked[0] != cmd.PostID {
		t.Errorf("tx calls = %d, locked = %v; want 1 tx with the post locked", tx.calls, feeds.locked)
	}
}

func TestFanOutPrivateOnlyReachesAuthor(t *testing.T) {
	svc, _, feeds := newFanOut(uuid.New())
	cmd := input.FanOutCommand{PostID: uuid.New(), AuthorID: uuid.New(), PublishedAt: time.Now(), Visibility: "private"}

	if err := svc.FanOut(context.Background(), cmd); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	got := recipients(feeds.inserted)
	if len(got) != 1 || !got[cmd.AuthorID] {
		t.Errorf("recipients = %v, want only the author", got)
	}
	if len(feeds.authorPosts) != 0 {
		t.Errorf("author_posts saves = %d, want 0 for a private post", len(feeds.authorPosts))
	}
}

func TestFanOutSkipsDeletedPost(t *testing.T) {
	svc, _, feeds := newFanOut(uuid.New())
	cmd := input.FanOutCommand{PostID: uuid.New(), AuthorID: uuid.New(), PublishedAt: time.Now(), Visibility: "public"}
	feeds.deleted[cmd.PostID] = true // posts.deleted procesado antes que posts.created

	if err := svc.FanOut(context.Background(), cmd); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(feeds.inserted) != 0 || len(feeds.authorPosts) != 0 {
		t.Errorf("inserted %d entries and %d author_posts, want none for a deleted post",
			len(feeds.inserted), len(feeds.authorPosts))
	}
}
