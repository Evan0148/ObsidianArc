package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

func TestAdministratorPageGrants(t *testing.T) {
	in := newInstance(t)
	founder := in.register("founder", "a-good-password")
	operator := in.register("operator", "a-good-password")
	pages := map[string]string{
		"dashboard": "/dashboard", "users": "/users", "groups": "/groups",
		"providers": "/providers", "models": "/models", "availability": "/health",
		"usage": "/usage", "resources": "/resources", "codes": "/codes",
		"logs": "/logs", "security": "/security/events", "settings": "/settings",
		"announcements": "/announcements", "administrators": "/administrators",
	}
	for _, grant := range user.AdminPermissions {
		t.Run(grant, func(t *testing.T) {
			response := in.do(http.MethodPatch, "/api/admin/administrators/"+operator.userID,
				map[string]any{"role": "admin", "admin_permissions": []string{grant}}, founder)
			if response.Code != http.StatusOK {
				t.Fatalf("grant: %d %s", response.Code, response.Body.String())
			}
			for page, path := range pages {
				response = in.do(http.MethodGet, "/api/admin"+path, nil, operator)
				allowed := page == grant || (page == "settings" && (grant == "security" || grant == "availability"))
				if allowed {
					if response.Code != http.StatusOK {
						t.Errorf("%s: %d %s", page, response.Code, response.Body.String())
					}
				} else if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "admin_permission_denied") {
					t.Errorf("ungranted %s: %d %s", page, response.Code, response.Body.String())
				}
			}
		})
	}
}

func TestDelegatedAdministratorCannotIncreaseAuthority(t *testing.T) {
	in := newInstance(t)
	founder := in.register("founder", "a-good-password")
	operator := in.register("operator", "a-good-password")
	other := in.register("other", "a-good-password")
	grant := func(target *session, role string, permissions []string, actor *session, status int) {
		t.Helper()
		res := in.do(http.MethodPatch, "/api/admin/administrators/"+target.userID,
			map[string]any{"role": role, "admin_permissions": permissions}, actor)
		if res.Code != status {
			t.Fatalf("grant %s as %s: %d %s", target.userID, actor.userID, res.Code, res.Body.String())
		}
	}
	grant(operator, "admin", []string{"administrators", "users", "groups"}, founder, http.StatusOK)
	grant(other, "admin", []string{"users"}, operator, http.StatusOK)
	grant(other, "admin", []string{"settings"}, operator, http.StatusForbidden)
	grant(other, "super_admin", []string{}, operator, http.StatusForbidden)
	grant(operator, "admin", []string{"users"}, operator, http.StatusForbidden)
	grant(founder, "user", []string{}, operator, http.StatusForbidden)
	grant(founder, "user", []string{}, founder, http.StatusConflict)
	grant(other, "admin", []string{"unknown"}, founder, http.StatusBadRequest)
	for _, operation := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPatch, "/api/admin/users/" + founder.userID, map[string]any{"email": "changed@example.com"}},
		{http.MethodDelete, "/api/admin/users/" + founder.userID, nil},
		{http.MethodPost, "/api/admin/users/" + founder.userID + "/password", map[string]any{"new_password": "replacement-password"}},
	} {
		res := in.do(operation.method, operation.path, operation.body, operator)
		if res.Code != http.StatusForbidden {
			t.Fatalf("protected administrator: %d %s", res.Code, res.Body.String())
		}
	}
	grant(operator, "admin", []string{"administrators"}, founder, http.StatusOK)
	res := in.do(http.MethodPatch, "/api/admin/administrators/"+other.userID, map[string]any{"nickname": "changed"}, operator)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("role page edited profile: %d", res.Code)
	}
	grant(other, "admin", []string{}, operator, http.StatusForbidden)
	res = in.do(http.MethodGet, "/api/admin/users", nil, operator)
	if res.Code != http.StatusForbidden {
		t.Fatalf("revoked permission still usable: %d", res.Code)
	}
}

func TestSettingsGrantsAreScopedBySection(t *testing.T) {
	in := newInstance(t)
	founder := in.register("founder", "a-good-password")
	operator := in.register("operator", "a-good-password")
	for _, grant := range []string{"security", "availability", "settings"} {
		res := in.do(http.MethodPatch, "/api/admin/administrators/"+operator.userID,
			map[string]any{"role": "admin", "admin_permissions": []string{grant}}, founder)
		if res.Code != http.StatusOK {
			t.Fatal(res.Body.String())
		}
		for key, section := range map[string]string{"site.name": "settings", "registration.enabled": "security", "health.probe": "availability"} {
			value := "false"
			if key == "site.name" {
				value = "Arc"
			}
			res = in.do(http.MethodPut, "/api/admin/settings", map[string]string{key: value}, operator)
			want := http.StatusForbidden
			if section == grant {
				want = http.StatusOK
			}
			if res.Code != want {
				t.Fatalf("%s writes %s: %d %s", grant, key, res.Code, res.Body.String())
			}
		}
		res = in.do(http.MethodGet, "/api/admin/settings", nil, operator)
		payload := decode[struct {
			Settings map[string]string `json:"settings"`
		}](t, res)
		for key := range payload.Settings {
			section := "settings"
			if strings.HasPrefix(key, "health.") {
				section = "availability"
			}
			if strings.HasPrefix(key, "registration.") || strings.HasPrefix(key, "turnstile.") || strings.HasPrefix(key, "security.") {
				section = "security"
			}
			if section != grant {
				t.Errorf("%s received %s", grant, key)
			}
		}
	}
}

func TestAdminUserPagesReachEveryAccount(t *testing.T) {
	in := newInstance(t)
	founder := in.register("founder", "a-good-password")
	store := user.NewStore(in.db)
	for i := 0; i < 65; i++ {
		if _, err := store.Create(context.Background(), nil, user.CreateInput{Username: fmt.Sprintf("page-%02d", i), PasswordHash: "unused"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, endpoint := range []string{"users", "member-options", "administrators"} {
		seen := map[string]bool{}
		for offset := 0; offset < 65; offset += 20 {
			res := in.do(http.MethodGet, fmt.Sprintf("/api/admin/%s?q=page-&limit=20&offset=%d", endpoint, offset), nil, founder)
			if res.Code != http.StatusOK {
				t.Fatal(res.Body.String())
			}
			payload := decode[struct {
				Users []user.User `json:"users"`
				Total int         `json:"total"`
			}](t, res)
			if payload.Total != 65 {
				t.Fatalf("%s total = %d", endpoint, payload.Total)
			}
			for _, account := range payload.Users {
				if seen[account.ID] {
					t.Fatalf("%s duplicate page row %s", endpoint, account.ID)
				}
				seen[account.ID] = true
			}
		}
		if len(seen) != 65 {
			t.Fatalf("%s only reached %d accounts", endpoint, len(seen))
		}
	}
}
