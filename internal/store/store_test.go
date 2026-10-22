package store

import (
	"context"
	"log/slog"
	"testing"

	"github.com/unmaykr-a/transit-tracker/internal/store/db"
)

func TestOpenMigratesAndTx(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, dir, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Q.SetMeta(ctx, db.SetMetaParams{Key: "k", Value: "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Q.SetMeta(ctx, db.SetMetaParams{Key: "k", Value: "v2"}); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Q.GetMeta(ctx, "k"); err != nil || v != "v2" {
		t.Fatalf("meta = %q, %v", v, err)
	}
	// A failing transaction leaves nothing behind.
	err = s.Tx(ctx, func(q *db.Queries) error {
		if err := q.InsertRoute(ctx, db.InsertRouteParams{ID: "tram:T4", Mode: "tram", ShortName: "T4", Source: "test"}); err != nil {
			return err
		}
		// Pattern referencing a missing route violates the foreign key.
		return q.InsertPattern(ctx, db.InsertPatternParams{ID: "p", RouteID: "nope", StopIds: "[]"})
	})
	if err == nil {
		t.Fatal("foreign key violation not reported")
	}
	if rs, _ := s.Q.ListRoutes(ctx); len(rs) != 0 {
		t.Fatalf("rolled-back route persisted: %v", rs)
	}
	s.Close()
	// Reopening an existing database is a no-op migration.
	s2, err := Open(ctx, dir, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
}
