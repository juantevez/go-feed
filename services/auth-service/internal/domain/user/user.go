package user

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	emailRegex    = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,50}$`)
)

// ErrUserNotFound is returned when a user lookup yields no result.
var ErrUserNotFound = errors.New("user not found")

// ErrEmailAlreadyTaken is returned when the email is already registered.
var ErrEmailAlreadyTaken = errors.New("email already taken")

// ErrInvalidCredentials is returned on authentication failure.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ValidationError is returned when User data breaks an invariant (bad username,
// bad email...). It's the client's fault: adapters map it to 400 with Msg.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string { return e.Msg }

func invalid(field, msg string) error { return &ValidationError{Field: field, Msg: msg} }

// Role represents the authorization level of a user.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// User is the aggregate root of the auth bounded context.
// It encapsulates identity and profile data; no infrastructure concerns live here.
type User struct {
	ID           uuid.UUID
	Username     string
	Email        string
	PasswordHash string
	Role         Role
	IsVerified   bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NormalizeEmail es la forma canónica de un email: sin espacios y en minúsculas.
// Se aplica al guardar y al buscar, así "Juan@X.com" y "juan@x.com" son la misma cuenta.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// New creates and validates a User aggregate.
// The caller is responsible for hashing the password before passing it in.
func New(username, email, passwordHash string) (*User, error) {
	u := &User{
		ID:           uuid.New(),
		Username:     strings.TrimSpace(username),
		Email:        NormalizeEmail(email),
		PasswordHash: passwordHash,
		Role:         RoleUser,
		IsVerified:   false,
		CreatedAt:    time.Now().UTC(),
	}
	u.UpdatedAt = u.CreatedAt

	if err := u.Validate(); err != nil {
		return nil, err
	}
	return u, nil
}

// Validate checks all invariants of the User aggregate.
// Called on construction and can be re-used in update operations.
func (u *User) Validate() error {
	if u.Username == "" {
		return invalid("username", "username is required")
	}
	if !usernameRegex.MatchString(u.Username) {
		return invalid("username", "username must be 3–50 characters: letters, numbers or underscores")
	}
	if u.Email == "" {
		return invalid("email", "email is required")
	}
	if !emailRegex.MatchString(u.Email) {
		return invalid("email", "email format is invalid")
	}
	if u.PasswordHash == "" {
		// Lo genera el use case, no el cliente: es un error interno, no de validación.
		return errors.New("password hash is required")
	}
	return nil
}
