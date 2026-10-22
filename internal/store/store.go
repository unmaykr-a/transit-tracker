// Package store owns the SQLite database: connection, embedded goose
// migrations, and the sqlc-generated queries in store/db.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGO, builds for any GOARCH)

	"github.com/unmaykr-a/transit-tracker/internal/store/db"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store wraps the database handle and generated queries.
type Store struct {
	DB *sql.DB
	Q  *db.Queries
}

// Open opens (creating if needed) dir/tram.db and applies pending migrations.
func Open(ctx context.Context, dir string, log *slog.Logger) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("data dir: %w", err)
	}
	return OpenDSN(ctx, "file:"+filepath.Join(dir, "tram.db"), log)
}

// OpenDSN opens a database by DSN (tests use "file::memory:?cache=shared" style DSNs).
func OpenDSN(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	sep := "?"
	if containsQuery(dsn) {
		sep = "&"
	}
	// WAL lets the HTTP handlers read while an import writes; busy_timeout makes
	// concurrent writers wait instead of failing.
	dsn += sep + "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}
	goose.SetBaseFS(migrations)
	goose.SetLogger(gooseLogger{log})
	if err := goose.SetDialect("sqlite3"); err != nil {
		conn.Close()
		return nil, err
	}
	if err := goose.UpContext(ctx, conn, "migrations"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{DB: conn, Q: db.New(conn)}, nil
}

func containsQuery(s string) bool {
	for i := range len(s) {
		if s[i] == '?' {
			return true
		}
	}
	return false
}

func (s *Store) Close() error { return s.DB.Close() }

// Tx runs fn in a transaction, committing on success.
func (s *Store) Tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(s.Q.WithTx(tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

type gooseLogger struct{ log *slog.Logger }

func (g gooseLogger) Fatalf(format string, v ...any) { g.log.Error(fmt.Sprintf(format, v...)) }
func (g gooseLogger) Printf(format string, v ...any) { g.log.Debug(fmt.Sprintf(format, v...)) }
