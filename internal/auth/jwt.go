package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/nglong14/llmgateway/internal/config"
)

var (
	ErrInvalidToken = errors.New("auth: invalid token")
)

// Claims are the JWT claims for access and refresh tokens.
type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	jwt.RegisteredClaims
}

// TokenManager issues and validates access/refresh JWTs.
type TokenManager struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

// NewTokenManager builds a TokenManager from JWT config.
func NewTokenManager(cfg config.JWTConfig) (*TokenManager, error) {
	if cfg.AccessTokenSecret == "" {
		return nil, fmt.Errorf("auth: access_token_secret is required")
	}
	if cfg.RefreshTokenSecret == "" {
		return nil, fmt.Errorf("auth: refresh_token_secret is required")
	}
	accessTTL := cfg.AccessTokenTTL
	if accessTTL <= 0 {
		accessTTL = 15 * time.Minute
	}
	refreshTTL := cfg.RefreshTokenTTL
	if refreshTTL <= 0 {
		refreshTTL = 720 * time.Hour
	}
	return &TokenManager{
		accessSecret:  []byte(cfg.AccessTokenSecret),
		refreshSecret: []byte(cfg.RefreshTokenSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
	}, nil
}

func (tm *TokenManager) AccessTTL() time.Duration { return tm.accessTTL }

func (tm *TokenManager) RefreshTTL() time.Duration { return tm.refreshTTL }

func (tm *TokenManager) GenerateAccessToken(userID uuid.UUID, email string) (token string, expiresAt time.Time, err error) {
	return tm.generate(userID, email, tm.accessSecret, tm.accessTTL)
}

func (tm *TokenManager) GenerateRefreshToken(userID uuid.UUID, email string) (token string, expiresAt time.Time, err error) {
	return tm.generate(userID, email, tm.refreshSecret, tm.refreshTTL)
}

func (tm *TokenManager) ValidateAccessToken(tokenString string) (*Claims, error) {
	return tm.validate(tokenString, tm.accessSecret)
}

func (tm *TokenManager) ValidateRefreshToken(tokenString string) (*Claims, error) {
	return tm.validate(tokenString, tm.refreshSecret)
}

func (tm *TokenManager) generate(userID uuid.UUID, email string, secret []byte, ttl time.Duration) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        uuid.New().String(),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, expiresAt, nil
}

func (tm *TokenManager) validate(tokenString string, secret []byte) (*Claims, error) {
	if tokenString == "" {
		return nil, ErrInvalidToken
	}
	tok, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil || !tok.Valid {
		return nil, ErrInvalidToken
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || claims.UserID == uuid.Nil {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
