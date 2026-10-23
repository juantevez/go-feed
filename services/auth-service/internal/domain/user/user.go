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

// New creates and validates a User aggregate.
// The caller is responsible for hashing the password before passing it in.
func New(username, email, passwordHash string) (*User, error) {
	u := &User{
		ID:           uuid.New(),
		Username:     strings.TrimSpace(username),
		Email:        strings.ToLower(strings.TrimSpace(email)),
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
		return errors.New("username is required")
	}
	if !usernameRegex.MatchString(u.Username) {
		return errors.New("username must be 3–50 characters: letters, numbers or underscores")
	}
	if u.Email == "" {
		return errors.New("email is required")
	}
	if !emailRegex.MatchString(u.Email) {
		return errors.New("email format is invalid")
	}
	if u.PasswordHash == "" {
		return errors.New("password hash is required")
	}
	return nil
}
