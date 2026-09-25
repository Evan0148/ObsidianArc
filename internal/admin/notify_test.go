package admin

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/config"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/database"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/group"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/notify"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// notifyFixture is the small slice of the wiring notifyQuotaReset actually
// touches, rather than the whole of Handlers: standing up every store
// NewHandlers takes for one method that reads none of them would be testing
// the fixture more than the code.
func notifyFixture(t *testing.T) (*Handlers, *notify.Store, string) {
	t.Helper()
	ctx := context.Background()

	db, err := database.Open(ctx, config.Database{
		Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "admin-notify.db"),
		MaxOpenConns: 4, MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	groups := group.NewStore(db)
	if _, err := groups.Create(ctx, nil, group.CreateInput{Name: "Default", IsDefault: true}); err != nil {
		t.Fatalf("create group: %v", err)
	}
	users := user.NewStore(db)
	account, err := users.Create(ctx, nil, user.CreateInput{Username: "reset-target", PasswordHash: "x"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	notifyStore := notify.NewStore(db)
	return &Handlers{Notify: notifyStore}, notifyStore, account.ID
}

// A global reset reaches everyone; a single account's reset reaches only
// that account — matching the scopes resetQuota itself distinguishes.
func TestQuotaResetNotifiesTheRightAudience(t *testing.T) {
	h, notifyStore, targetID := notifyFixture(t)
	ctx := context.Background()

	h.notifyQuotaReset(ctx, notify.Notification{Audience: notify.AudienceUser, UserID: targetID})

	everyone := user.User{ID: "someone-else", CreatedAt: 0}
	if notices, err := notifyStore.List(ctx, everyone, 10, 0); err != nil || len(notices) != 0 {
		t.Fatalf("an unrelated account's notices = %+v (err %v), want none from a user-scoped reset", notices, err)
	}
	target := user.User{ID: targetID}
	notices, err := notifyStore.List(ctx, target, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(notices) != 1 || notices[0].Kind != "quota_reset" || notices[0].Link != "/usage" {
		t.Fatalf("notices = %+v, want one quota_reset for the reset account", notices)
	}

	h.notifyQuotaReset(ctx, notify.Notification{Audience: notify.AudienceAll})
	if notices, err := notifyStore.List(ctx, everyone, 10, 0); err != nil || len(notices) != 1 {
		t.Fatalf("notices after a global reset = %+v (err %v), want the broadcast to reach everyone", notices, err)
	}

	// A nil Notify — every instance before this feature — must not panic.
	bare := &Handlers{}
	bare.notifyQuotaReset(ctx, notify.Notification{Audience: notify.AudienceAll})
}
