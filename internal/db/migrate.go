package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

var migrationNameRE = regexp.MustCompile(`^(\d+)_(.+)\.(up|down)\.sql$`)

// Migration describes a single versioned up/down pair.
type Migration struct {
	Version int64
	Name    string
	UpSQL   string
	DownSQL string
}

// Status summarizes applied vs pending migrations.
type Status struct {
	CurrentVersion int64
	Applied        []Migration
	Pending        []Migration
}

// Migrator applies and rolls back embedded SQL migrations.
type Migrator struct {
	pool *pgxpool.Pool
	fs   embed.FS
}

// NewMigrator creates a migrator bound to the given pool.
func NewMigrator(pool *pgxpool.Pool) *Migrator {
	return &Migrator{pool: pool, fs: migrationFS}
}

// Up applies all pending migrations in version order.
func (m *Migrator) Up(ctx context.Context) (int, error) {
	if err := m.ensureTable(ctx); err != nil {
		return 0, err
	}
	migrations, err := loadMigrations(m.fs)
	if err != nil {
		return 0, err
	}
	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return 0, err
	}
	if err := validateOrder(migrations, applied); err != nil {
		return 0, err
	}

	count := 0
	for _, mig := range migrations {
		if _, ok := applied[mig.Version]; ok {
			continue
		}
		if err := m.applyUp(ctx, mig); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// Down rolls back the most recently applied migration.
func (m *Migrator) Down(ctx context.Context) error {
	if err := m.ensureTable(ctx); err != nil {
		return err
	}
	migrations, err := loadMigrations(m.fs)
	if err != nil {
		return err
	}
	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return err
	}
	if err := validateOrder(migrations, applied); err != nil {
		return err
	}
	if len(applied) == 0 {
		return fmt.Errorf("db/migrate: no migrations to roll back")
	}

	var latest int64
	for v := range applied {
		if v > latest {
			latest = v
		}
	}

	var mig *Migration
	for i := range migrations {
		if migrations[i].Version == latest {
			mig = &migrations[i]
			break
		}
	}
	if mig == nil {
		return fmt.Errorf("db/migrate: applied version %d has no matching migration file", latest)
	}
	if strings.TrimSpace(mig.DownSQL) == "" {
		return fmt.Errorf("db/migrate: migration %d (%s) has empty down SQL", mig.Version, mig.Name)
	}

	return m.applyDown(ctx, *mig)
}

// Status returns the current schema version and pending migrations.
func (m *Migrator) Status(ctx context.Context) (*Status, error) {
	if err := m.ensureTable(ctx); err != nil {
		return nil, err
	}
	migrations, err := loadMigrations(m.fs)
	if err != nil {
		return nil, err
	}
	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateOrder(migrations, applied); err != nil {
		return nil, err
	}

	st := &Status{}
	for _, mig := range migrations {
		if _, ok := applied[mig.Version]; ok {
			st.Applied = append(st.Applied, mig)
			if mig.Version > st.CurrentVersion {
				st.CurrentVersion = mig.Version
			}
		} else {
			st.Pending = append(st.Pending, mig)
		}
	}
	return st, nil
}

func (m *Migrator) ensureTable(ctx context.Context) error {
	_, err := m.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("db/migrate: create schema_migrations: %w", err)
	}
	return nil
}

func (m *Migrator) appliedVersions(ctx context.Context) (map[int64]time.Time, error) {
	rows, err := m.pool.Query(ctx, `SELECT version, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("db/migrate: list applied: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]time.Time)
	for rows.Next() {
		var version int64
		var appliedAt time.Time
		if err := rows.Scan(&version, &appliedAt); err != nil {
			return nil, fmt.Errorf("db/migrate: scan applied: %w", err)
		}
		out[version] = appliedAt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db/migrate: iterate applied: %w", err)
	}
	return out, nil
}

func (m *Migrator) applyUp(ctx context.Context, mig Migration) error {
	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("db/migrate: begin up %d: %w", mig.Version, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, mig.UpSQL); err != nil {
		return fmt.Errorf("db/migrate: apply up %d (%s): %w", mig.Version, mig.Name, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, mig.Version); err != nil {
		return fmt.Errorf("db/migrate: record up %d: %w", mig.Version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db/migrate: commit up %d: %w", mig.Version, err)
	}
	return nil
}

func (m *Migrator) applyDown(ctx context.Context, mig Migration) error {
	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("db/migrate: begin down %d: %w", mig.Version, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, mig.DownSQL); err != nil {
		return fmt.Errorf("db/migrate: apply down %d (%s): %w", mig.Version, mig.Name, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, mig.Version); err != nil {
		return fmt.Errorf("db/migrate: record down %d: %w", mig.Version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db/migrate: commit down %d: %w", mig.Version, err)
	}
	return nil
}

func loadMigrations(fsys embed.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return nil, fmt.Errorf("db/migrate: read migrations: %w", err)
	}

	type partial struct {
		version int64
		name    string
		up      string
		down    string
	}
	byVersion := make(map[int64]*partial)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		matches := migrationNameRE.FindStringSubmatch(e.Name())
		if matches == nil {
			return nil, fmt.Errorf("db/migrate: invalid migration filename %q", e.Name())
		}
		version, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("db/migrate: invalid version in %q: %w", e.Name(), err)
		}
		name := matches[2]
		direction := matches[3]

		p, ok := byVersion[version]
		if !ok {
			p = &partial{version: version, name: name}
			byVersion[version] = p
		} else if p.name != name {
			return nil, fmt.Errorf("db/migrate: version %d has conflicting names %q and %q", version, p.name, name)
		}

		body, err := fs.ReadFile(fsys, path.Join("migrations", e.Name()))
		if err != nil {
			return nil, fmt.Errorf("db/migrate: read %s: %w", e.Name(), err)
		}
		switch direction {
		case "up":
			if p.up != "" {
				return nil, fmt.Errorf("db/migrate: duplicate up for version %d", version)
			}
			p.up = string(body)
		case "down":
			if p.down != "" {
				return nil, fmt.Errorf("db/migrate: duplicate down for version %d", version)
			}
			p.down = string(body)
		}
	}

	migrations := make([]Migration, 0, len(byVersion))
	for _, p := range byVersion {
		if strings.TrimSpace(p.up) == "" {
			return nil, fmt.Errorf("db/migrate: version %d (%s) missing up SQL", p.version, p.name)
		}
		if strings.TrimSpace(p.down) == "" {
			return nil, fmt.Errorf("db/migrate: version %d (%s) missing down SQL", p.version, p.name)
		}
		migrations = append(migrations, Migration{
			Version: p.version,
			Name:    p.name,
			UpSQL:   p.up,
			DownSQL: p.down,
		})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations, nil
}

// validateOrder ensures applied versions form a contiguous prefix of known migrations.
func validateOrder(migrations []Migration, applied map[int64]time.Time) error {
	known := make(map[int64]struct{}, len(migrations))
	for _, mig := range migrations {
		known[mig.Version] = struct{}{}
	}
	for version := range applied {
		if _, ok := known[version]; !ok {
			return fmt.Errorf("db/migrate: unknown applied version %d (no matching migration file)", version)
		}
	}
	// Applied versions must be a prefix: once a gap appears, nothing later may be applied.
	seenGap := false
	for _, mig := range migrations {
		_, isApplied := applied[mig.Version]
		if !isApplied {
			seenGap = true
			continue
		}
		if seenGap {
			return fmt.Errorf("db/migrate: applied version %d is out of order (gap in migration history)", mig.Version)
		}
	}
	return nil
}
