package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("sqlite mkdir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite pragmas: %w", err)
	}
	migrations, err := fs.Sub(migrationFS, "migrations")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite migration fs: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrations)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
