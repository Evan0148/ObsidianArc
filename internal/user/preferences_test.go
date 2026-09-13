package user

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/config"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/database"
)

func TestBehaviouralBooleanPreferenceIsStrict(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{
		Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "preferences.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx,
		`INSERT INTO users (id, username, username_lower, password_hash, role, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"reader", "reader", "reader", "x", RoleUser, StatusActive, 1, 1); err != nil {
		t.Fatal(err)
	}

	store := NewPreferenceStore(db)
	if enabled, err := store.Bool(ctx, "reader", AutoUseResetCard); err != nil || enabled {
		t.Fatalf("empty preference = %v, %v", enabled, err)
	}
	if _, err := store.Merge(ctx, "reader", map[string]json.RawMessage{
		AutoUseResetCard: json.RawMessage(`true`),
	}); err != nil {
		t.Fatal(err)
	}
	if enabled, err := store.Bool(ctx, "reader", AutoUseResetCard); err != nil || !enabled {
		t.Fatalf("saved preference = %v, %v", enabled, err)
	}
	if _, err := store.Merge(ctx, "reader", map[string]json.RawMessage{
		AutoUseResetCard: json.RawMessage(`"true"`),
	}); err != nil {
		t.Fatal(err)
	}
	if enabled, err := store.Bool(ctx, "reader", AutoUseResetCard); err != nil || enabled {
		t.Fatalf("string preference = %v, %v", enabled, err)
	}
}
