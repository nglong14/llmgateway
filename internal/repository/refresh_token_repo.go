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

// RefreshTokenRepository persists refresh token hashes.
type RefreshTokenRepository struct {
	pool *pgxpool.Pool
}

func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{pool: pool}
}

func (r *RefreshTokenRepository) Insert(ctx context.Context, userID, familyID uuid.UUID, tokenHash string, expiresAt time.Time) (*models.RefreshToken, error) {
	const q = `
		INSERT INTO refresh_tokens (user_id, token_hash, family_id, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, token_hash, family_id, expires_at, revoked_at, created_at
	`
	var t models.RefreshToken
	err := r.pool.QueryRow(ctx, q, userID, tokenHash, familyID, expiresAt).Scan(
		&t.ID, &t.UserID, &t.TokenHash, &t.FamilyID, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("refresh_token_repo: insert: %w", err)
	}
	return &t, nil
}

func (r *RefreshTokenRepository) GetByHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	const q = `
		SELECT id, user_id, token_hash, family_id, expires_at, revoked_at, created_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`
	var t models.RefreshToken
	err := r.pool.QueryRow(ctx, q, tokenHash).Scan(
		&t.ID, &t.UserID, &t.TokenHash, &t.FamilyID, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("refresh_token_repo: get by hash: %w", err)
	}
	return &t, nil
}

func (r *RefreshTokenRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = NOW()
		WHERE id = $1 AND revoked_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("refresh_token_repo: revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeFamily marks all non-revoked tokens in a rotation family as revoked.
func (r *RefreshTokenRepository) RevokeFamily(ctx context.Context, familyID uuid.UUID) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = NOW()
		WHERE family_id = $1 AND revoked_at IS NULL
	`, familyID)
	if err != nil {
		return 0, fmt.Errorf("refresh_token_repo: revoke family: %w", err)
	}
	return tag.RowsAffected(), nil
}
