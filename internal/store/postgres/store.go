package postgres

import (
	"context"
	"embed"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ductringuyen-0618/feature-flag-api/internal/flag"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store"
)

// schema.sql is the sole DDL source for this store (go:embed). Apply on connect
// with CREATE IF NOT EXISTS; there is no separate migrations/ runner.
//
//go:embed schema.sql
var schemaFS embed.FS

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	s := &Store{pool: pool}
	if err := s.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) ensureSchema(ctx context.Context) error {
	sql, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, string(sql))
	return err
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) Close() error {
	s.pool.Close()
	return nil
}

func (s *Store) CreateFlag(ctx context.Context, f flag.Flag) (flag.Flag, error) {
	now := time.Now().UTC()
	row := s.pool.QueryRow(ctx, `
		INSERT INTO flags (name, description, enabled, rollout_percent, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		RETURNING name, description, enabled, rollout_percent, created_at, updated_at`,
		f.Name, f.Description, f.Enabled, f.RolloutPercent, now,
	)
	out, err := scanFlag(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return flag.Flag{}, store.ErrAlreadyExists
		}
		return flag.Flag{}, err
	}
	return out, nil
}

func (s *Store) GetFlag(ctx context.Context, name string) (flag.Flag, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT name, description, enabled, rollout_percent, created_at, updated_at
		FROM flags WHERE name = $1`, name)
	out, err := scanFlag(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return flag.Flag{}, store.ErrNotFound
	}
	return out, err
}

func (s *Store) ListFlags(ctx context.Context) ([]flag.Flag, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT name, description, enabled, rollout_percent, created_at, updated_at
		FROM flags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []flag.Flag
	for rows.Next() {
		var f flag.Flag
		if err := rows.Scan(&f.Name, &f.Description, &f.Enabled, &f.RolloutPercent, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if out == nil {
		out = []flag.Flag{}
	}
	return out, rows.Err()
}

func (s *Store) UpdateFlag(ctx context.Context, name string, enabled *bool, description *string, rollout *int) (flag.Flag, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE flags SET
			enabled = COALESCE($2, enabled),
			description = COALESCE($3, description),
			rollout_percent = COALESCE($4, rollout_percent),
			updated_at = NOW()
		WHERE name = $1
		RETURNING name, description, enabled, rollout_percent, created_at, updated_at`,
		name, enabled, description, rollout,
	)
	out, err := scanFlag(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return flag.Flag{}, store.ErrNotFound
	}
	return out, err
}

func (s *Store) DeleteFlag(ctx context.Context, name string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM flags WHERE name = $1`, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) SetOverride(ctx context.Context, o flag.Override) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_overrides (flag_name, user_id, enabled)
		VALUES ($1, $2, $3)
		ON CONFLICT (flag_name, user_id) DO UPDATE SET enabled = EXCLUDED.enabled`,
		o.FlagName, o.UserID, o.Enabled,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return store.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) GetOverride(ctx context.Context, flagName, userID string) (flag.Override, error) {
	var o flag.Override
	err := s.pool.QueryRow(ctx, `
		SELECT flag_name, user_id, enabled FROM user_overrides
		WHERE flag_name = $1 AND user_id = $2`, flagName, userID,
	).Scan(&o.FlagName, &o.UserID, &o.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return flag.Override{}, store.ErrNotFound
	}
	return o, err
}

func (s *Store) DeleteOverride(ctx context.Context, flagName, userID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM user_overrides WHERE flag_name = $1 AND user_id = $2`, flagName, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanFlag(row scannable) (flag.Flag, error) {
	var f flag.Flag
	err := row.Scan(&f.Name, &f.Description, &f.Enabled, &f.RolloutPercent, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

var _ store.FlagStore = (*Store)(nil)

