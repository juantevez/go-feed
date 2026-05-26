package domain

import "context"

type User struct {
	ID       string
	Username string
	Email    string
}

type UserStore interface {
	Create(ctx context.Context, u *User, hashedPass []byte) error
	FindByEmail(ctx context.Context, email string) (*struct {
		ID       string
		Username string
		Email    string
		Password []byte
	}, error)
}
