package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nglong14/llmgateway/internal/models"
)

// APIKeyRepository persists API keys.
type APIKeyRepository struct {
	pool *pgxpool.Pool
}

func NewAPIKeyRepository(pool *pgxpool.Pool) *APIKeyRepository {
	return &APIKeyRepository{pool: pool}
}

func (r *APIKeyRepository) Create(ctx context.Context, userID uuid.UUID, keyPrefix, keyHash, name string) (*models.APIKey, error) {
	const q = `
		INSERT INTO api_keys (user_id, key_prefix, key_hash, name)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, key_prefix, key_hash, name, last_used_at, created_at, revoked_at
	`
	var k models.APIKey
	err := r.pool.QueryRow(ctx, q, userID, keyPrefix, keyHash, name).Scan(
		&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.LastUsedAt, &k.CreatedAt, &k.RevokedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("apikey_repo: create: %w", err)
	}
	return &k, nil
}

func (r *APIKeyRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.APIKey, error) {
	const q = `
		SELECT id, user_id, key_prefix, key_hash, name, last_used_at, created_at, revoked_at
		FROM api_keys WHERE id = $1
	`
	return r.scanOne(ctx, q, id)
}

// GetByHash returns a non-revoked key matching the hash.
func (r *APIKeyRepository) GetByHash(ctx context.Context, keyHash string) (*models.APIKey, error) {
	const q = `
		SELECT id, user_id, key_prefix, key_hash, name, last_used_at, created_at, revoked_at
		FROM api_keys
		WHERE key_hash = $1 AND revoked_at IS NULL
	`
	return r.scanOne(ctx, q, keyHash)
}

func (r *APIKeyRepository) ListByUserID(ctx context.Context, userID uuid.UUID) ([]models.APIKey, error) {
	const q = `
		SELECT id, user_id, key_prefix, key_hash, name, last_used_at, created_at, revoked_at
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("apikey_repo: list: %w", err)
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(
			&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.LastUsedAt, &k.CreatedAt, &k.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("apikey_repo: scan: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("apikey_repo: iterate: %w", err)
	}
	if keys == nil {
		keys = []models.APIKey{}
	}
	return keys, nil
}

func (r *APIKeyRepository) TouchLastUsed(ctx context.Context, id uuid.UUID, at time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE api_keys SET last_used_at = $2
		WHERE id = $1 AND revoked_at IS NULL
	`, id, at)
	if err != nil {
		return fmt.Errorf("apikey_repo: touch last used: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *APIKeyRepository) Revoke(ctx context.Context, id, userID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE api_keys SET revoked_at = NOW()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
	`, id, userID)
	if err != nil {
		return fmt.Errorf("apikey_repo: revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *APIKeyRepository) scanOne(ctx context.Context, q string, args ...any) (*models.APIKey, error) {
	var k models.APIKey
	err := r.pool.QueryRow(ctx, q, args...).Scan(
		&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.LastUsedAt, &k.CreatedAt, &k.RevokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("apikey_repo: get: %w", err)
	}
	return &k, nil
}
