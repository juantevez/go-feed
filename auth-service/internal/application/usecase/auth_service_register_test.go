package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/auth-service/internal/application/port/input"
	"github.com/juantevez/my-ig/auth-service/internal/application/usecase"
	"github.com/juantevez/my-ig/auth-service/internal/domain/user"
	"github.com/juantevez/my-ig/shared/events"
)

// ── Mocks ────────────────────────────────────────────────────────────────────

type mockUserRepo struct {
	existsByEmail  func(ctx context.Context, email string) (bool, error)
	save           func(ctx context.Context, u *user.User) error
	findByEmail    func(ctx context.Context, email string) (*user.User, error)
	findByID       func(ctx context.Context, id uuid.UUID) (*user.User, error)
	findByUsername func(ctx context.Context, username string) (*user.User, error)
	update         func(ctx context.Context, u *user.User) error
}

func (m *mockUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return m.existsByEmail(ctx, email)
}
func (m *mockUserRepo) Save(ctx context.Context, u *user.User) error {
	return m.save(ctx, u)
}
func (m *mockUserRepo) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	if m.findByEmail != nil {
		return m.findByEmail(ctx, email)
	}
	return nil, user.ErrUserNotFound
}
func (m *mockUserRepo) FindByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	if m.findByID != nil {
		return m.findByID(ctx, id)
	}
	return nil, user.ErrUserNotFound
}
func (m *mockUserRepo) FindByUsername(ctx context.Context, username string) (*user.User, error) {
	if m.findByUsername != nil {
		return m.findByUsername(ctx, username)
	}
	return nil, user.ErrUserNotFound
}
func (m *mockUserRepo) Update(ctx context.Context, u *user.User) error {
	if m.update != nil {
		return m.update(ctx, u)
	}
	return nil
}

type mockPublisher struct {
	published []struct {
		topic   string
		payload any
	}
	err error
}

func (m *mockPublisher) Publish(_ context.Context, topic string, payload any) error {
	m.published = append(m.published, struct {
		topic   string
		payload any
	}{topic, payload})
	return m.err
}

// token.Service nil stub — Register no lo usa todavía.
type nilTokenService struct{}

func (nilTokenService) GeneratePair(_ context.Context, _ uuid.UUID, _ string) (*interface{}, error) {
	return nil, nil
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestRegister_Success(t *testing.T) {
	pub := &mockPublisher{}
	repo := &mockUserRepo{
		existsByEmail: func(_ context.Context, _ string) (bool, error) { return false, nil },
		save:          func(_ context.Context, _ *user.User) error { return nil },
	}

	svc := usecase.NewAuthService(repo, nil, pub)

	result, err := svc.Register(context.Background(), input.RegisterCommand{
		Username: "juandev",
		Email:    "juan@example.com",
		Password: "supersecret123",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.UserID == uuid.Nil {
		t.Fatal("expected a non-nil UserID")
	}

	// El evento se publica en una goroutine; damos tiempo suficiente.
	time.Sleep(50 * time.Millisecond)
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 event published, got %d", len(pub.published))
	}
	if pub.published[0].topic != events.TopicAuthUserRegistered {
		t.Errorf("expected topic %q, got %q", events.TopicAuthUserRegistered, pub.published[0].topic)
	}
}

func TestRegister_EmailAlreadyTaken(t *testing.T) {
	repo := &mockUserRepo{
		existsByEmail: func(_ context.Context, _ string) (bool, error) { return true, nil },
	}
	svc := usecase.NewAuthService(repo, nil, &mockPublisher{})

	_, err := svc.Register(context.Background(), input.RegisterCommand{
		Username: "juandev",
		Email:    "taken@example.com",
		Password: "supersecret123",
	})

	if !errors.Is(err, user.ErrEmailAlreadyTaken) {
		t.Errorf("expected ErrEmailAlreadyTaken, got %v", err)
	}
}

func TestRegister_InvalidUsername(t *testing.T) {
	repo := &mockUserRepo{
		existsByEmail: func(_ context.Context, _ string) (bool, error) { return false, nil },
	}
	svc := usecase.NewAuthService(repo, nil, &mockPublisher{})

	_, err := svc.Register(context.Background(), input.RegisterCommand{
		Username: "u!", // falla la regex del dominio
		Email:    "juan@example.com",
		Password: "supersecret123",
	})

	if err == nil {
		t.Fatal("expected a validation error, got nil")
	}
}

func TestRegister_RepoSaveFails(t *testing.T) {
	dbErr := errors.New("connection refused")
	repo := &mockUserRepo{
		existsByEmail: func(_ context.Context, _ string) (bool, error) { return false, nil },
		save:          func(_ context.Context, _ *user.User) error { return dbErr },
	}
	svc := usecase.NewAuthService(repo, nil, &mockPublisher{})

	_, err := svc.Register(context.Background(), input.RegisterCommand{
		Username: "juandev",
		Email:    "juan@example.com",
		Password: "supersecret123",
	})

	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, dbErr) {
		t.Errorf("expected wrapped dbErr, got %v", err)
	}
}
