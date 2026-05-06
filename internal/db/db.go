package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"coremem/migrations"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	DB *sql.DB
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("database path is required")
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
	}
	dsn := path
	if path == ":memory:" {
		dsn = "file:coremem?mode=memory&cache=shared"
	}
	if !strings.Contains(dsn, "?") {
		dsn += "?"
	} else {
		dsn += "&"
	}
	dsn += "_foreign_keys=on&_busy_timeout=5000"
	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return &Store{DB: sqlDB}, nil
}

func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	if s == nil || s.DB == nil {
		return errors.New("nil store")
	}
	migs, err := migrations.All()
	if err != nil {
		return err
	}
	for _, mig := range migs {
		if err := s.applyMigration(ctx, mig.Name, mig.SQL); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, name, sqlText string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var existing string
	err = tx.QueryRowContext(ctx, `SELECT name FROM schema_migrations WHERE name = ?`, name).Scan(&existing)
	if err == nil {
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		// The first migration creates schema_migrations, so absence is expected.
		if _, createErr := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); createErr != nil {
			return createErr
		}
		err = tx.QueryRowContext(ctx, `SELECT name FROM schema_migrations WHERE name = ?`, name).Scan(&existing)
		if err == nil {
			return tx.Commit()
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, sqlText); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations (name, applied_at) VALUES (?, ?)`, name, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return tx.Commit()
}
