package input

import (
	"context"

	"github.com/google/uuid"
)

// --- Register ---

type RegisterCommand struct {
	Username string
	Email    string
	Password string // plain-text; hashing happens inside the use case
}

type RegisterResult struct {
	UserID uuid.UUID
}

// --- Login ---

type LoginCommand struct {
	Email    string
	Password string
}

type LoginResult struct {
	AccessToken  string
	RefreshToken string
}

// --- Refresh ---

type RefreshCommand struct {
	RefreshToken string
}

type RefreshResult struct {
	AccessToken  string
	RefreshToken string
}

// --- Logout ---

type LogoutCommand struct {
	JTI string // JTI of the access token being invalidated
}

// AuthUseCase is the primary (driving) port.
// The HTTP handler depends on this interface, never on concrete use cases.
type AuthUseCase interface {
	Register(ctx context.Context, cmd RegisterCommand) (*RegisterResult, error)
	Login(ctx context.Context, cmd LoginCommand) (*LoginResult, error)
	Refresh(ctx context.Context, cmd RefreshCommand) (*RefreshResult, error)
	Logout(ctx context.Context, cmd LogoutCommand) error
}
