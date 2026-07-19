package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nglong14/llmgateway/internal/models"
)

var (
	// ErrNotFound is returned when a row does not exist.
	ErrNotFound = errors.New("repository: not found")
	// ErrConflict is returned on unique constraint violations.
	ErrConflict = errors.New("repository: conflict")
)

// UserRepository persists users.
type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, email, passwordHash, name string) (*models.User, error) {
	const q = `
		INSERT INTO users (email, password_hash, name)
		VALUES ($1, $2, $3)
		RETURNING id, email, password_hash, name, created_at, updated_at
	`
	var u models.User
	err := r.pool.QueryRow(ctx, q, email, passwordHash, name).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("user_repo: create: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	const q = `
		SELECT id, email, password_hash, name, created_at, updated_at
		FROM users WHERE id = $1
	`
	return r.scanOne(ctx, q, id)
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	const q = `
		SELECT id, email, password_hash, name, created_at, updated_at
		FROM users WHERE email = $1
	`
	return r.scanOne(ctx, q, email)
}

func (r *UserRepository) Update(ctx context.Context, id uuid.UUID, name, passwordHash string) (*models.User, error) {
	const q = `
		UPDATE users
		SET name = $2,
		    password_hash = $3,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, email, password_hash, name, created_at, updated_at
	`
	var u models.User
	err := r.pool.QueryRow(ctx, q, id, name, passwordHash).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user_repo: update: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("user_repo: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *UserRepository) scanOne(ctx context.Context, q string, args ...any) (*models.User, error) {
	var u models.User
	err := r.pool.QueryRow(ctx, q, args...).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user_repo: get: %w", err)
	}
	return &u, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
