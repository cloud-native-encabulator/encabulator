// Package store persists feature flags in Postgres.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/flags"
)

// ErrNotFound is returned when a flag key does not exist.
var ErrNotFound = errors.New("flag not found")

// Store is the flag table.
type Store struct {
	pool *pgxpool.Pool
}

// New connects to Postgres with OTel instrumentation on the pool.
func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse DATABASE_URL: %w", err)
	}
	cfg.ConnConfig.Tracer = otelpgx.NewTracer(otelpgx.WithTrimSQLInSpanName())
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Pool exposes the connection pool for migrations.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// Ping is the readiness check.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Seed is one built-in flag definition.
type Seed struct {
	Key         string
	Type        string
	Value       any
	Description string
}

// Seed inserts the built-in flags if they are missing, and never overwrites an
// operator's edit.
func (s *Store) Seed(ctx context.Context, seeds []Seed) error {
	for _, seed := range seeds {
		raw, err := json.Marshal(seed.Value)
		if err != nil {
			return fmt.Errorf("store: encode seed %s: %w", seed.Key, err)
		}
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO flags (key, type, value, description, updated_by)
			VALUES ($1, $2, $3, $4, 'seed')
			ON CONFLICT (key) DO NOTHING`,
			seed.Key, seed.Type, raw, seed.Description,
		); err != nil {
			return fmt.Errorf("store: seed %s: %w", seed.Key, err)
		}
	}
	return nil
}

const selectFlags = `
	SELECT key, type, value, rollout_percent, description, updated_at, updated_by
	FROM flags`

// List returns every flag, ordered by key so that the ETag is stable.
func (s *Store) List(ctx context.Context) ([]flags.Flag, error) {
	rows, err := s.pool.Query(ctx, selectFlags+` ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("store: list flags: %w", err)
	}
	defer rows.Close()

	out := make([]flags.Flag, 0, 8)
	for rows.Next() {
		f, err := scanFlag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list flags: %w", err)
	}
	return out, nil
}

// Get returns one flag, or ErrNotFound.
func (s *Store) Get(ctx context.Context, key string) (flags.Flag, error) {
	rows, err := s.pool.Query(ctx, selectFlags+` WHERE key = $1`, key)
	if err != nil {
		return flags.Flag{}, fmt.Errorf("store: get flag %s: %w", key, err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return flags.Flag{}, fmt.Errorf("store: get flag %s: %w", key, err)
		}
		return flags.Flag{}, ErrNotFound
	}
	return scanFlag(rows)
}

// Update is a flag edit. A nil Description leaves the existing description.
type Update struct {
	Value          json.RawMessage
	RolloutPercent *int
	Description    *string
	UpdatedBy      string
}

// Put applies an update to an existing flag and returns the stored result.
// Unknown keys return ErrNotFound: the flag set is the one in the design, not
// whatever a caller invents.
func (s *Store) Put(ctx context.Context, key string, up Update) (flags.Flag, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE flags
		   SET value = $2,
		       rollout_percent = $3,
		       description = COALESCE($4, description),
		       updated_at = now(),
		       updated_by = $5
		 WHERE key = $1
		RETURNING key, type, value, rollout_percent, description, updated_at, updated_by`,
		key, up.Value, up.RolloutPercent, up.Description, up.UpdatedBy)
	if err != nil {
		return flags.Flag{}, fmt.Errorf("store: update flag %s: %w", key, err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return flags.Flag{}, fmt.Errorf("store: update flag %s: %w", key, err)
		}
		return flags.Flag{}, ErrNotFound
	}
	return scanFlag(rows)
}

func scanFlag(rows pgx.Rows) (flags.Flag, error) {
	var (
		f       flags.Flag
		value   []byte
		rollout *int32
	)
	if err := rows.Scan(&f.Key, &f.Type, &value, &rollout, &f.Description, &f.UpdatedAt, &f.UpdatedBy); err != nil {
		return flags.Flag{}, fmt.Errorf("store: scan flag: %w", err)
	}
	f.Value = json.RawMessage(value)
	if rollout != nil {
		pct := int(*rollout)
		f.RolloutPercent = &pct
	}
	f.UpdatedAt = f.UpdatedAt.UTC()
	return f, nil
}
