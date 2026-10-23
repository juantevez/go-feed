package usecase

import (
	"context"
	"fmt"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"

	"github.com/juantevez/my-ig/auth-service/internal/application/port/input"
	"github.com/juantevez/my-ig/auth-service/internal/application/port/output"
	"github.com/juantevez/my-ig/auth-service/internal/domain/token"
	"github.com/juantevez/my-ig/auth-service/internal/domain/user"
	"github.com/juantevez/my-ig/shared/events"
)

const bcryptCost = 12

// AuthService implements input.AuthUseCase.
type AuthService struct {
	tx        output.TxManager
	users     user.Repository
	tokens    token.Service
	publisher output.EventPublisher
}

func NewAuthService(
	tx output.TxManager,
	users user.Repository,
	tokens token.Service,
	publisher output.EventPublisher,
) *AuthService {
	return &AuthService{tx: tx, users: users, tokens: tokens, publisher: publisher}
}

// Register creates a new user account and hashes the password.
// The user and the auth.user.registered.v1 event are persisted in the same tx (outbox).
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

	traceID := chimiddleware.GetReqID(ctx)
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.users.Save(ctx, u); err != nil {
			return fmt.Errorf("register: save: %w", err)
		}
		if err := s.publisher.Publish(ctx, events.TopicAuthUserRegistered, user.NewRegisteredEnvelope(u, traceID)); err != nil {
			return fmt.Errorf("register: enqueue event: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &input.RegisterResult{UserID: u.ID}, nil
}

// Login verifies credentials and returns a fresh JWT pair.
// The refresh token and the auth.user.logged_in.v1 event are persisted in the same tx (outbox).
func (s *AuthService) Login(ctx context.Context, cmd input.LoginCommand) (*input.LoginResult, error) {
	u, err := s.users.FindByEmail(ctx, cmd.Email)
	if err != nil {
		// Don't leak whether the email exists — always return same error.
		return nil, user.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(cmd.Password)); err != nil {
		return nil, user.ErrInvalidCredentials
	}

	var pair *token.Pair
	traceID := chimiddleware.GetReqID(ctx)
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		pair, err = s.tokens.GeneratePair(ctx, u.ID, string(u.Role))
		if err != nil {
			return fmt.Errorf("login: generate tokens: %w", err)
		}
		if err := s.publisher.Publish(ctx, events.TopicAuthUserLoggedIn, user.NewLoggedInEnvelope(u, traceID)); err != nil {
			return fmt.Errorf("login: enqueue event: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

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
