package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"golang.org/x/crypto/bcrypt"

	"github.com/juantevez/my-ig/auth-service/internal/application/port/input"
	"github.com/juantevez/my-ig/auth-service/internal/application/port/output"
	"github.com/juantevez/my-ig/auth-service/internal/domain/token"
	"github.com/juantevez/my-ig/auth-service/internal/domain/user"
)

const bcryptCost = 12

// AuthService implements input.AuthUseCase.
type AuthService struct {
	users     user.Repository
	tokens    token.Service
	publisher output.EventPublisher
}

func NewAuthService(
	users user.Repository,
	tokens token.Service,
	publisher output.EventPublisher,
) *AuthService {
	return &AuthService{users: users, tokens: tokens, publisher: publisher}
}

// Register creates a new user account, hashes the password and publishes
// the auth.user.registered.v1 event (fire-and-forget).
func (s *AuthService) Register(ctx context.Context, cmd input.RegisterCommand) (*input.RegisterResult, error) {
	taken, err := s.users.ExistsByEmail(ctx, cmd.Email)
	if err != nil {
		return nil, fmt.Errorf("register: check email: %w", err)
	}
	if taken {
		return nil, user.ErrEmailAlreadyTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cmd.Password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("register: hash password: %w", err)
	}

	u, err := user.New(cmd.Username, cmd.Email, string(hash))
	if err != nil {
		return nil, fmt.Errorf("register: build user: %w", err)
	}

	if err := s.users.Save(ctx, u); err != nil {
		return nil, fmt.Errorf("register: save: %w", err)
	}

	s.publishAsync(user.TopicRegistered, user.NewRegisteredEvent(u), u.ID.String())

	return &input.RegisterResult{UserID: u.ID}, nil
}

// Login verifies credentials and returns a fresh JWT pair.
// Publishes auth.user.logged_in.v1 on success.
func (s *AuthService) Login(ctx context.Context, cmd input.LoginCommand) (*input.LoginResult, error) {
	u, err := s.users.FindByEmail(ctx, cmd.Email)
	if err != nil {
		// Don't leak whether the email exists — always return same error.
		return nil, user.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(cmd.Password)); err != nil {
		return nil, user.ErrInvalidCredentials
	}

	pair, err := s.tokens.GeneratePair(ctx, u.ID, string(u.Role))
	if err != nil {
		return nil, fmt.Errorf("login: generate tokens: %w", err)
	}

	s.publishAsync(user.TopicLoggedIn, user.NewLoggedInEvent(u), u.ID.String())

	return &input.LoginResult{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
	}, nil
}

// Refresh validates the refresh token and rotates the pair.
func (s *AuthService) Refresh(ctx context.Context, cmd input.RefreshCommand) (*input.RefreshResult, error) {
	pair, err := s.tokens.Refresh(ctx, cmd.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("refresh: %w", err)
	}
	return &input.RefreshResult{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
	}, nil
}

// Logout revokes the access token identified by its JTI.
func (s *AuthService) Logout(ctx context.Context, cmd input.LogoutCommand) error {
	if err := s.tokens.Revoke(ctx, cmd.JTI); err != nil {
		return fmt.Errorf("logout: revoke token: %w", err)
	}
	return nil
}

// ── internal ─────────────────────────────────────────────────────────────────

// publishAsync emits a domain event in a background goroutine.
// A failure here never rolls back the operation — consumers reconcile from DB.
func (s *AuthService) publishAsync(topic string, event any, userID string) {
	go func() {
		if err := s.publisher.Publish(context.Background(), topic, event); err != nil {
			slog.Error("event publish failed",
				"topic", topic,
				"user_id", userID,
				"err", err,
			)
		}
	}()
}
