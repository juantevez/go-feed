package usecase

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"feed-backend/services/auth/internal/domain"
)

type JWTConfig struct {
	Secret     []byte
	Expiration time.Duration
}

type AuthUseCase struct {
	store domain.UserStore
	jwt   JWTConfig
}

func NewAuthUseCase(store domain.UserStore) *AuthUseCase {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-secret-change-me-in-prod"
	}
	return &AuthUseCase{
		store: store,
		jwt:   JWTConfig{Secret: []byte(secret), Expiration: 15 * time.Minute},
	}
}

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	UserID       string `json:"user_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"` // Placeholder para fase 2
	ExpiresIn    int64  `json:"expires_in"`
}

func (uc *AuthUseCase) Register(ctx context.Context, req RegisterRequest) (*AuthResponse, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &domain.User{
		ID:       generateID(), // En prod: uuid.New().String()
		Username: req.Username,
		Email:    req.Email,
	}

	if err := uc.store.Create(ctx, user, hash); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	token, exp := uc.generateToken(user.ID, user.Username)
	return &AuthResponse{
		UserID:      user.ID,
		AccessToken: token,
		ExpiresIn:   exp,
	}, nil
}

func (uc *AuthUseCase) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	dbUser, err := uc.store.FindByEmail(ctx, req.Email)
	if err != nil {
		// Seguridad: no diferenciar entre "no existe" y "password incorrecta"
		return nil, fmt.Errorf("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword(dbUser.Password, []byte(req.Password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	token, exp := uc.generateToken(dbUser.ID, dbUser.Username)
	return &AuthResponse{
		UserID:      dbUser.ID,
		AccessToken: token,
		ExpiresIn:   exp,
	}, nil
}

func (uc *AuthUseCase) generateToken(userID, username string) (string, int64) {
	now := time.Now()
	exp := now.Add(uc.jwt.Expiration)

	claims := jwt.MapClaims{
		"sub":      userID,
		"username": username,
		"iat":      now.Unix(),
		"exp":      exp.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	t, _ := token.SignedString(uc.jwt.Secret)
	return t, int64(uc.jwt.Expiration.Seconds())
}

// Helper temporal para evitar importar uuid en este ejemplo
func generateID() string {
	b := make([]byte, 16)
	// En producción: import "github.com/google/uuid" y usar uuid.New().String()
	for i := range b {
		b[i] = byte(i)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
