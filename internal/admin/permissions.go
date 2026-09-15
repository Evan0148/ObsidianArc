package admin

import (
	"net/http"
	"strings"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/auth"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/httpx"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/quota"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

func permissionDenied() error {
	return httpx.ForbiddenCode("admin_permission_denied", "You do not have permission to access this page or perform this action.")
}

func hasPermission(account user.User, permissions string) bool {
	if permissions == "" {
		return account.IsAdmin()
	}
	for _, permission := range strings.Split(permissions, ",") {
		if account.CanAdmin(permission) {
			return true
		}
	}
	return false
}

func settingPermission(key string) string {
	switch {
	case strings.HasPrefix(key, "health."):
		return "availability"
	case strings.HasPrefix(key, "registration."), strings.HasPrefix(key, "turnstile."), strings.HasPrefix(key, "security."):
		return "security"
	default:
		return "settings"
	}
}

func canPolicy(actor user.User, scope quota.Scope) bool {
	if actor.CanAdmin("usage") {
		return true
	}
	return (scope == quota.ScopeUser && actor.CanAdmin("users")) ||
		(scope == quota.ScopeGroup && actor.CanAdmin("groups"))
}

func (h *Handlers) visibleSettings(account user.User) map[string]string {
	values := redacted(h.settings.All())
	for key := range values {
		if !account.CanAdmin(settingPermission(key)) {
			delete(values, key)
		}
	}
	return values
}

// Forms need names from neighbouring pages. This endpoint deliberately
// returns just selector data, without granting access to those pages' records.
func (h *Handlers) references(w http.ResponseWriter, r *http.Request) error {
	actor := auth.MustUser(r.Context())
	out := map[string]any{}
	if hasPermission(actor, "users,models,usage,security,settings,groups") {
		groups, err := h.groups.List(r.Context(), nil)
		if err != nil {
			return httpx.Internal(err)
		}
		items := make([]map[string]any, 0, len(groups))
		for _, g := range groups {
			items = append(items, map[string]any{"id": g.ID, "name": g.Name})
		}
		out["groups"] = items
	}
	if hasPermission(actor, "groups,settings,security") {
		models, err := h.models.ListAll(r.Context(), "")
		if err != nil {
			return httpx.Internal(err)
		}
		items := make([]map[string]any, 0, len(models))
		for _, m := range models {
			items = append(items, map[string]any{"id": m.ID, "display_name": m.DisplayName, "model_id": m.ModelID, "enabled": m.Enabled, "provider_name": m.ProviderName})
		}
		out["models"] = items
	}
	if actor.CanAdmin("models") {
		providers, err := h.providers.List(r.Context())
		if err != nil {
			return httpx.Internal(err)
		}
		items := make([]map[string]any, 0, len(providers))
		for _, p := range providers {
			items = append(items, map[string]any{"id": p.ID, "name": p.Name, "kind": p.Kind, "enabled": p.Enabled})
		}
		out["providers"] = items
	}
	return httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) listMemberOptions(w http.ResponseWriter, r *http.Request) error {
	query := r.URL.Query()
	accounts, total, err := h.users.List(r.Context(), user.ListFilter{
		Search: query.Get("q"), GroupID: query.Get("group_id"),
		Limit: intParam(query.Get("limit"), 20), Offset: intParam(query.Get("offset"), 0),
	})
	if err != nil {
		return httpx.Internal(err)
	}
	items := make([]map[string]any, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, map[string]any{"id": account.ID, "username": account.Username, "nickname": account.Nickname, "group_id": account.GroupID, "group_expires_at": account.GroupExpiresAt})
	}
	return httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": items, "total": total})
}
