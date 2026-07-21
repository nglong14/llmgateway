package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nglong14/llmgateway/internal/models"
	"github.com/nglong14/llmgateway/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

const (
	apiKeyBytes      = 32
	apiKeyPrefixLen  = 11 // "gw-" + 8 chars of payload
	bcryptCost       = bcrypt.DefaultCost
	defaultKeyName   = "default"
)

var (
	ErrEmailTaken = errors.New("auth: email already taken")
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrTokenReuse = errors.New("auth: refresh token reuse detected")
	ErrKeyNotFound = errors.New("auth: api key not found")
)

type userStore interface {
	Create(ctx context.Context, email, passwordHash, name string) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)
}

type apiKeyStore interface {
	Create(ctx context.Context, userID uuid.UUID, keyPrefix, keyHash, name string) (*models.APIKey, error)
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]models.APIKey, error)
	Revoke(ctx context.Context, id, userID uuid.UUID) error
}

type refreshStore interface {
	Insert(ctx context.Context, userID, familyID uuid.UUID, tokenHash string, expiresAt time.Time) (*models.RefreshToken, error)
	GetByHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error)
	Revoke(ctx context.Context, id uuid.UUID) error
	RevokeFamily(ctx context.Context, familyID uuid.UUID) (int64, error)
}

type Service struct {
	users   userStore
	keys    apiKeyStore
	refresh refreshStore
	tokens  *TokenManager
}

// NewService wires repositories and a token manager into an auth service.
func NewService(
	users *repository.UserRepository,
	keys *repository.APIKeyRepository,
	refresh *repository.RefreshTokenRepository,
	tokens *TokenManager,
) *Service {
	return &Service{users: users, keys: keys, refresh: refresh, tokens: tokens}
}

type SignUpResult struct {
	User   *models.User
	APIKey string
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	TokenType    string
}

type CreateAPIKeyResult struct {
	Key     *models.APIKey
	APIKey  string
}

// SignUp creates a user and auto-generates their first API key.
func (s *Service) SignUp(ctx context.Context, email, password, name string) (*SignUpResult, error) {
	email = normalizeEmail(email)
	if email == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}

	user, err := s.users.Create(ctx, email, hash, name)
	if err != nil {
		if errors.Is(err, repository.ErrConflict) {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("auth: signup: %w", err)
	}

	plaintext, prefix, keyHash, err := generateAPIKey()
	if err != nil {
		return nil, err
	}
	if _, err := s.keys.Create(ctx, user.ID, prefix, keyHash, defaultKeyName); err != nil {
		return nil, fmt.Errorf("auth: signup create key: %w", err)
	}

	return &SignUpResult{User: user, APIKey: plaintext}, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (*models.User, error) {
	return s.authenticate(ctx, email, password)
}

func (s *Service) IssueTokens(ctx context.Context, email, password string) (*TokenPair, error) {
	user, err := s.authenticate(ctx, email, password)
	if err != nil {
		return nil, err
	}
	return s.issuePair(ctx, user, uuid.New())
}

func (s *Service) RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.tokens.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	stored, err := s.refresh.GetByHash(ctx, hashToken(refreshToken))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("auth: refresh lookup: %w", err)
	}

	if stored.RevokedAt != nil {
		_, _ = s.refresh.RevokeFamily(ctx, stored.FamilyID)
		return nil, ErrTokenReuse
	}
	if time.Now().UTC().After(stored.ExpiresAt) {
		_ = s.refresh.Revoke(ctx, stored.ID)
		return nil, ErrInvalidToken
	}
	if stored.UserID != claims.UserID {
		return nil, ErrInvalidToken
	}

	user, err := s.users.GetByID(ctx, stored.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("auth: refresh user: %w", err)
	}

	if err := s.refresh.Revoke(ctx, stored.ID); err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("auth: revoke old refresh: %w", err)
	}

	return s.issuePair(ctx, user, stored.FamilyID)
}

func (s *Service) CreateAPIKey(ctx context.Context, userID uuid.UUID, name string) (*CreateAPIKeyResult, error) {
	if name == "" {
		name = defaultKeyName
	}
	plaintext, prefix, keyHash, err := generateAPIKey()
	if err != nil {
		return nil, err
	}
	key, err := s.keys.Create(ctx, userID, prefix, keyHash, name)
	if err != nil {
		return nil, fmt.Errorf("auth: create api key: %w", err)
	}
	return &CreateAPIKeyResult{Key: key, APIKey: plaintext}, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, userID uuid.UUID) ([]models.APIKey, error) {
	keys, err := s.keys.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("auth: list api keys: %w", err)
	}
	return keys, nil
}

func (s *Service) RevokeAPIKey(ctx context.Context, userID, keyID uuid.UUID) error {
	err := s.keys.Revoke(ctx, keyID, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrKeyNotFound
	}
	if err != nil {
		return fmt.Errorf("auth: revoke api key: %w", err)
	}
	return nil
}

func (s *Service) authenticate(ctx context.Context, email, password string) (*models.User, error) {
	email = normalizeEmail(email)
	if email == "" || password == "" {
		return nil, ErrInvalidCredentials
	}
	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("auth: lookup user: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return user, nil
}

func (s *Service) issuePair(ctx context.Context, user *models.User, familyID uuid.UUID) (*TokenPair, error) {
	access, _, err := s.tokens.GenerateAccessToken(user.ID, user.Email)
	if err != nil {
		return nil, err
	}
	refresh, refreshExp, err := s.tokens.GenerateRefreshToken(user.ID, user.Email)
	if err != nil {
		return nil, err
	}
	if _, err := s.refresh.Insert(ctx, user.ID, familyID, hashToken(refresh), refreshExp); err != nil {
		return nil, fmt.Errorf("auth: persist refresh: %w", err)
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(s.tokens.AccessTTL().Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func hashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(b), nil
}

func generateAPIKey() (plaintext, prefix, keyHash string, err error) {
	raw := make([]byte, apiKeyBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", "", fmt.Errorf("auth: generate api key: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	plaintext = "gw-" + payload
	prefix = plaintext
	if len(prefix) > apiKeyPrefixLen {
		prefix = prefix[:apiKeyPrefixLen]
	}
	sum := sha256.Sum256([]byte(plaintext))
	keyHash = hex.EncodeToString(sum[:])
	return plaintext, prefix, keyHash, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
