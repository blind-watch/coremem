package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"coremem/internal/db"
)

func TestMigrateCreatesSchema(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "coremem.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := store.DB.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='memory_nodes'`).Scan(&name); err != nil {
		t.Fatalf("memory_nodes table not created: %v", err)
	}
	if name != "memory_nodes" {
		t.Fatalf("unexpected table %q", name)
	}
}
