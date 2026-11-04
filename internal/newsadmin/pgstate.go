package newsadmin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StateDB keeps the service state document (served build, revision, history,
// push results) in Postgres instead of <data>/state.json. The builds/ and
// channels/ trees stay files: they are operator-published content.
type StateDB struct{ pool *pgxpool.Pool }

const stateKey = "state"

const stateSchema = `CREATE TABLE IF NOT EXISTS news_state (
  key        text PRIMARY KEY,
  doc        jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
)`

// OpenStateDB connects (small pool) and creates the table.
func OpenStateDB(ctx context.Context, url string) (*StateDB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse state database url: %w", err)
	}
	cfg.MaxConns = 3
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(c, stateSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &StateDB{pool: pool}, nil
}

func (d *StateDB) Close() { d.pool.Close() }

func (d *StateDB) load() ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var b []byte
	err := d.pool.QueryRow(ctx, `SELECT doc::text FROM news_state WHERE key=$1`, stateKey).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	return b, err == nil, err
}

func (d *StateDB) save(b []byte, overwrite bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	q := `INSERT INTO news_state(key,doc) VALUES($1,$2::jsonb) ON CONFLICT (key) DO NOTHING`
	if overwrite {
		q = `INSERT INTO news_state(key,doc) VALUES($1,$2::jsonb) ON CONFLICT (key) DO UPDATE SET doc=EXCLUDED.doc, updated_at=now()`
	}
	_, err := d.pool.Exec(ctx, q, stateKey, string(b))
	return err
}

// readState returns the stored document. With a database it is filled once
// from the legacy state.json when the database has none (the file is kept).
func (s *Service) readState() ([]byte, bool, error) {
	if s.cfg.StateDB == nil {
		raw, err := os.ReadFile(s.statePath())
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return raw, err == nil, err
	}
	raw, ok, err := s.cfg.StateDB.load()
	if err != nil || ok {
		return raw, ok, err
	}
	fb, ferr := os.ReadFile(s.statePath())
	if ferr != nil {
		return nil, false, nil
	}
	if err := s.cfg.StateDB.save(fb, false); err != nil {
		return nil, false, fmt.Errorf("import state.json: %w", err)
	}
	s.log.Info("imported state.json into postgres (file kept)", "bytes", len(fb))
	raw, ok, err = s.cfg.StateDB.load()
	if err == nil && !ok {
		err = errors.New("imported state not readable back")
	}
	return raw, ok, err
}

func (s *Service) writeState(raw []byte) error {
	if s.cfg.StateDB == nil {
		return writeAtomic(s.statePath(), raw)
	}
	return s.cfg.StateDB.save(raw, true)
}
