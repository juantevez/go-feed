package usecase

import (
	"context"

	"github.com/tuusuario/my-ig/auth-service/internal/application/port/input"
	"github.com/tuusuario/my-ig/auth-service/internal/application/port/output"
	"github.com/tuusuario/my-ig/auth-service/internal/domain/token"
	"github.com/tuusuario/my-ig/auth-service/internal/domain/user"
)

// AuthService implements input.AuthUseCase.
// It orchestrates domain logic without depending on any infrastructure detail.
type AuthService struct {
	users     user.Repository
	tokens    token.Service
	publisher output.EventPublisher
}

// NewAuthService wires all driven ports into the service.
func NewAuthService(
	users user.Repository,
	tokens token.Service,
	publisher output.EventPublisher,
) *AuthService {
	return &AuthService{
		users:     users,
		tokens:    tokens,
		publisher: publisher,
	}
}

// Register creates a new user account.
// TODO: hash password, persist, publish auth.user.registered.v1
func (s *AuthService) Register(ctx context.Context, cmd input.RegisterCommand) (*input.RegisterResult, error) {
	panic("not implemented")
}

// Login authenticates a user and returns a JWT pair.
// TODO: verify credentials, generate token pair, publish auth.user.logged_in.v1
func (s *AuthService) Login(ctx context.Context, cmd input.LoginCommand) (*input.LoginResult, error) {
	panic("not implemented")
}

// Refresh rotates the token pair given a valid refresh token.
// TODO: delegate to token.Service.Refresh, publish auth.token.refreshed.v1
func (s *AuthService) Refresh(ctx context.Context, cmd input.RefreshCommand) (*input.RefreshResult, error) {
	panic("not implemented")
}

// Logout revokes the active token.
// TODO: delegate to token.Service.Revoke
func (s *AuthService) Logout(ctx context.Context, cmd input.LogoutCommand) error {
	panic("not implemented")
}
