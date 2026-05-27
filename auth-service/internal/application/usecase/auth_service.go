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

const bcryptCost = 12 // argon2 sería ideal en producción; bcrypt es suficiente para v1

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

// Register creates a new user account:
//  1. Valida que el email no esté tomado (fail-fast antes de hashear)
//  2. Hashea el password con bcrypt
//  3. Construye y persiste el agregado User
//  4. Publica auth.user.registered.v1 (fire-and-forget: no bloquea la respuesta)
func (s *AuthService) Register(ctx context.Context, cmd input.RegisterCommand) (*input.RegisterResult, error) {
	// 1. Fail-fast: evitamos el costo de bcrypt si el email ya existe.
	taken, err := s.users.ExistsByEmail(ctx, cmd.Email)
	if err != nil {
		return nil, fmt.Errorf("register: check email: %w", err)
	}
	if taken {
		return nil, user.ErrEmailAlreadyTaken
	}

	// 2. Hash del password — nunca sale del use case en texto plano.
	hash, err := bcrypt.GenerateFromPassword([]byte(cmd.Password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("register: hash password: %w", err)
	}

	// 3. Construir el agregado (valida username, email, etc. internamente).
	u, err := user.New(cmd.Username, cmd.Email, string(hash))
	if err != nil {
		return nil, fmt.Errorf("register: build user: %w", err)
	}

	// 4. Persistir.
	if err := s.users.Save(ctx, u); err != nil {
		return nil, fmt.Errorf("register: save user: %w", err)
	}

	// 5. Publicar evento de dominio — fire-and-forget con goroutine acotada.
	// Un fallo aquí NO revierte el registro: el consumer puede reconciliar
	// consultando la DB si necesita los datos del usuario.
	event := user.NewRegisteredEvent(u)
	go func() {
		if err := s.publisher.Publish(context.Background(), user.TopicRegistered, event); err != nil {
			slog.Error("register: publish event failed",
				"topic", user.TopicRegistered,
				"user_id", u.ID,
				"err", err,
			)
		}
	}()

	return &input.RegisterResult{UserID: u.ID}, nil
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
