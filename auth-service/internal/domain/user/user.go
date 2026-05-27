package user

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrUserNotFound is returned when a user lookup yields no result.
var ErrUserNotFound = errors.New("user not found")

// ErrEmailAlreadyTaken is returned when the email is already registered.
var ErrEmailAlreadyTaken = errors.New("email already taken")

// ErrInvalidCredentials is returned on authentication failure.
var ErrInvalidCredentials = errors.New("invalid credentials")

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

// New creates a valid User aggregate.
// The caller is responsible for hashing the password before passing it in.
func New(username, email, passwordHash string) (*User, error) {
	if username == "" {
		return nil, errors.New("username is required")
	}
	if email == "" {
		return nil, errors.New("email is required")
	}
	if passwordHash == "" {
		return nil, errors.New("password hash is required")
	}

	now := time.Now().UTC()
	return &User{
		ID:           uuid.New(),
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         RoleUser,
		IsVerified:   false,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}
