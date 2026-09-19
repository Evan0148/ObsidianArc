package server

import (
	"net/http"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/quota"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/settings"
)

func TestChatConcurrencySettingAndAdminExemption(t *testing.T) {
	in := newInstance(t)
	admin := in.register("admin_user", "password123")
	regular := in.register("regular_user", "password123")

	// 1. Initial setting defaults to 4.
	getRes := in.do(http.MethodGet, "/api/admin/settings", nil, admin)
	if getRes.Code != http.StatusOK {
		t.Fatalf("get settings: %d %s", getRes.Code, getRes.Body.String())
	}
	settingsBody := decode[map[string]any](t, getRes)
	values := settingsBody["settings"].(map[string]any)
	if values[settings.QuotaMaxConcurrent] != "4" {
		t.Errorf("default %s = %v, want 4", settings.QuotaMaxConcurrent, values[settings.QuotaMaxConcurrent])
	}

	// 2. Change quota.max_concurrent to 2.
	putRes := in.do(http.MethodPut, "/api/admin/settings", map[string]string{
		settings.QuotaMaxConcurrent: "2",
	}, admin)
	if putRes.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", putRes.Code, putRes.Body.String())
	}

	// 3. Verify server guard behavior for regular user: limited to 2.
	maxConcurrent := in.server.settings.Int(settings.QuotaMaxConcurrent, quota.DefaultMaxConcurrent)
	if maxConcurrent != 2 {
		t.Fatalf("server settings %s = %d, want 2", settings.QuotaMaxConcurrent, maxConcurrent)
	}

	rel1, err := in.server.quota.Begin(regular.userID, maxConcurrent)
	if err != nil {
		t.Fatalf("first regular slot: %v", err)
	}
	defer rel1()

	rel2, err := in.server.quota.Begin(regular.userID, maxConcurrent)
	if err != nil {
		t.Fatalf("second regular slot: %v", err)
	}
	defer rel2()

	_, err = in.server.quota.Begin(regular.userID, maxConcurrent)
	if err != quota.ErrTooManyInFlight {
		t.Fatalf("third regular slot err = %v, want ErrTooManyInFlight", err)
	}

	// 4. Administrator is exempt (maxConcurrent <= 0 means unlimited).
	adminSlots := make([]func(), 0, 10)
	for i := 0; i < 10; i++ {
		adminRel, err := in.server.quota.Begin(admin.userID, 0)
		if err != nil {
			t.Fatalf("admin slot %d: %v", i+1, err)
		}
		adminSlots = append(adminSlots, adminRel)
	}
	for _, rel := range adminSlots {
		rel()
	}

	// 5. Setting quota.max_concurrent to 0 makes regular users unlimited too.
	putRes = in.do(http.MethodPut, "/api/admin/settings", map[string]string{
		settings.QuotaMaxConcurrent: "0",
	}, admin)
	if putRes.Code != http.StatusOK {
		t.Fatalf("put settings zero: %d %s", putRes.Code, putRes.Body.String())
	}
	unlimited := in.server.settings.Int(settings.QuotaMaxConcurrent, quota.DefaultMaxConcurrent)
	for i := 0; i < 5; i++ {
		rel, err := in.server.quota.Begin(regular.userID, unlimited)
		if err != nil {
			t.Fatalf("unlimited regular slot %d: %v", i+1, err)
		}
		defer rel()
	}
}
