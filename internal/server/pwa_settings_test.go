package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/settings"
)

// The browser tab used to say "Obsidian Arc" on every instance, whatever an
// operator called their site. /api/site is where the client learns what to
// call the tab instead, already resolved against the site's own name so the
// client has no fallback of its own to keep in sync.
func TestSiteBrowserTitleFallsBackToSiteName(t *testing.T) {
	in := newInstance(t)
	admin := in.register("founder", "a-good-password")

	readTitle := func() string {
		t.Helper()
		response := in.do(http.MethodGet, "/api/site", nil, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET /api/site: %d %s", response.Code, response.Body.String())
		}
		var payload struct {
			BrowserTitle string `json:"browser_title"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload.BrowserTitle
	}

	// A fresh instance's name is "Obsidian Arc", and nobody has set a
	// separate browser title yet.
	if got := readTitle(); got != "Obsidian Arc" {
		t.Fatalf("browser_title = %q, want the default site name", got)
	}

	if response := in.do(http.MethodPut, "/api/admin/settings",
		map[string]string{settings.SiteName: "ACME Chat"}, admin); response.Code != http.StatusOK {
		t.Fatalf("rename site: %d %s", response.Code, response.Body.String())
	}
	if got := readTitle(); got != "ACME Chat" {
		t.Fatalf("browser_title = %q, want the renamed site to be reflected", got)
	}

	if response := in.do(http.MethodPut, "/api/admin/settings",
		map[string]string{settings.SiteBrowserTitle: "Internal Assistant"}, admin); response.Code != http.StatusOK {
		t.Fatalf("set browser title: %d %s", response.Code, response.Body.String())
	}
	if got := readTitle(); got != "Internal Assistant" {
		t.Fatalf("browser_title = %q, want the explicit title to win over the site name", got)
	}

	// Clearing it returns to following the site name, the way about.title
	// returns to the built-in wording when cleared.
	if response := in.do(http.MethodPut, "/api/admin/settings",
		map[string]string{settings.SiteBrowserTitle: ""}, admin); response.Code != http.StatusOK {
		t.Fatalf("clear browser title: %d %s", response.Code, response.Body.String())
	}
	if got := readTitle(); got != "ACME Chat" {
		t.Fatalf("browser_title = %q, want it to fall back to the site name again", got)
	}
}

// The manifest is what a browser reads to draw the "install" prompt. Public,
// like the shell itself, and reachable before anyone has signed in.
func TestManifestServesConfiguredValues(t *testing.T) {
	in := newInstance(t)
	admin := in.register("founder", "a-good-password")

	response := in.do(http.MethodPut, "/api/admin/settings", map[string]string{
		settings.PWAName:            "ACME App",
		settings.PWAShortName:       "ACME",
		settings.PWADescription:     "The internal chat assistant.",
		settings.PWAThemeColor:      "#112233",
		settings.PWABackgroundColor: "#445566",
		settings.PWAIconURL:         "/icon.png",
	}, admin)
	if response.Code != http.StatusOK {
		t.Fatalf("save PWA settings: %d %s", response.Code, response.Body.String())
	}

	manifest := in.do(http.MethodGet, "/manifest.webmanifest", nil, nil)
	if manifest.Code != http.StatusOK {
		t.Fatalf("GET /manifest.webmanifest: %d %s", manifest.Code, manifest.Body.String())
	}
	if ct := manifest.Header().Get("Content-Type"); ct != "application/manifest+json" {
		t.Errorf("Content-Type = %q, want application/manifest+json", ct)
	}

	var doc map[string]any
	if err := json.Unmarshal(manifest.Body.Bytes(), &doc); err != nil {
		t.Fatalf("manifest was not valid JSON: %v", err)
	}
	if doc["name"] != "ACME App" || doc["short_name"] != "ACME" {
		t.Errorf("name/short_name = %v / %v", doc["name"], doc["short_name"])
	}
	if doc["description"] != "The internal chat assistant." {
		t.Errorf("description = %v", doc["description"])
	}
	if doc["theme_color"] != "#112233" || doc["background_color"] != "#445566" {
		t.Errorf("colours = %v / %v", doc["theme_color"], doc["background_color"])
	}
	icons, _ := doc["icons"].([]any)
	if len(icons) != 1 || icons[0].(map[string]any)["src"] != "/icon.png" {
		t.Errorf("icons = %v", doc["icons"])
	}
}

// Nothing configured is the common case, and it must still produce a usable
// manifest: the site's own name and description, the interface's own default
// colours, and the built-in icon rather than a broken reference.
func TestManifestFallsBackToSiteIdentity(t *testing.T) {
	in := newInstance(t)
	admin := in.register("founder", "a-good-password")

	if response := in.do(http.MethodPut, "/api/admin/settings", map[string]string{
		settings.SiteName:        "ACME Chat",
		settings.SiteDescription: "Ask #it-help before filing a ticket.",
	}, admin); response.Code != http.StatusOK {
		t.Fatalf("save site identity: %d %s", response.Code, response.Body.String())
	}

	manifest := in.do(http.MethodGet, "/manifest.webmanifest", nil, nil)
	if manifest.Code != http.StatusOK {
		t.Fatalf("GET /manifest.webmanifest: %d %s", manifest.Code, manifest.Body.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(manifest.Body.Bytes(), &doc); err != nil {
		t.Fatalf("manifest was not valid JSON: %v", err)
	}
	if doc["name"] != "ACME Chat" || doc["short_name"] != "ACME Chat" {
		t.Errorf("name/short_name = %v / %v, want both to fall back to the site name", doc["name"], doc["short_name"])
	}
	if doc["description"] != "Ask #it-help before filing a ticket." {
		t.Errorf("description = %v, want the site description", doc["description"])
	}
	// The interface's own default neutral accent (color-utils.ts:
	// ACCENTS.neutral) — a fresh instance's install prompt is already
	// themed, not left white.
	if doc["theme_color"] != "#18181b" || doc["background_color"] != "#18181b" {
		t.Errorf("colours = %v / %v, want the built-in default", doc["theme_color"], doc["background_color"])
	}
	icons, _ := doc["icons"].([]any)
	if len(icons) != 1 {
		t.Fatalf("icons = %v, want exactly one", doc["icons"])
	}
	src, _ := icons[0].(map[string]any)["src"].(string)
	if src == "" || src == "/icon.png" {
		t.Errorf("icon src = %q, want the built-in inline mark", src)
	}
}

// A colour that is not #rrggbb would only be noticed when a browser silently
// ignored the whole manifest field, so it is refused at save time instead.
func TestPWAColorMustBeHex(t *testing.T) {
	in := newInstance(t)
	admin := in.register("founder", "a-good-password")

	for _, value := range []string{"blue", "#fff", "#gggggg", "18181b"} {
		response := in.do(http.MethodPut, "/api/admin/settings",
			map[string]string{settings.PWAThemeColor: value}, admin)
		if response.Code != http.StatusBadRequest {
			t.Errorf("theme colour %q: %d %s, want 400", value, response.Code, response.Body.String())
		}
	}

	// Valid, and empty (meaning "no override"), both succeed.
	for _, value := range []string{"#112233", ""} {
		response := in.do(http.MethodPut, "/api/admin/settings",
			map[string]string{settings.PWAThemeColor: value}, admin)
		if response.Code != http.StatusOK {
			t.Errorf("theme colour %q: %d %s", value, response.Code, response.Body.String())
		}
	}
}

// The manifest icon is declared with img-src 'self' data: blob:, so an
// operator pointing it at another host would only find out once a browser
// silently dropped the icon. Refusing it at save time says so immediately —
// mirroring web/src/lib/account.ts's safeAvatar, which refuses the same
// shapes for the same reason.
func TestPWAIconURLMustBeSameOriginOrInline(t *testing.T) {
	in := newInstance(t)
	admin := in.register("founder", "a-good-password")

	for _, value := range []string{"https://evil.example.com/icon.png", "//evil.example.com/icon.png", "javascript:alert(1)"} {
		response := in.do(http.MethodPut, "/api/admin/settings",
			map[string]string{settings.PWAIconURL: value}, admin)
		if response.Code != http.StatusBadRequest {
			t.Errorf("icon %q: %d %s, want 400", value, response.Code, response.Body.String())
		}
	}

	for _, value := range []string{"/icon.png", "data:image/png;base64,AAAA", ""} {
		response := in.do(http.MethodPut, "/api/admin/settings",
			map[string]string{settings.PWAIconURL: value}, admin)
		if response.Code != http.StatusOK {
			t.Errorf("icon %q: %d %s", value, response.Code, response.Body.String())
		}
	}
}

// An import is forgiving where a save refuses: a bad colour is dropped and
// named rather than costing the operator the rest of the document.
func TestImportSettingsDropsInvalidPWAColor(t *testing.T) {
	in := newInstance(t)
	admin := in.register("founder", "a-good-password")

	response := in.do(http.MethodPost, "/api/admin/settings/import", map[string]string{
		settings.PWAThemeColor: "not-a-colour",
		settings.SiteName:      "Carried Over",
	}, admin)
	if response.Code != http.StatusOK {
		t.Fatalf("import: %d %s", response.Code, response.Body.String())
	}
	result := decode[map[string]any](t, response)
	skipped, _ := result["skipped"].([]any)
	if len(skipped) != 1 || skipped[0] != settings.PWAThemeColor {
		t.Errorf("skipped = %v, want just %s", skipped, settings.PWAThemeColor)
	}

	saved := decode[map[string]any](t, in.do(http.MethodGet, "/api/admin/settings", nil, admin))
	values, _ := saved["settings"].(map[string]any)
	if values[settings.PWAThemeColor] != "#18181b" {
		t.Errorf("%s = %v, want the default kept", settings.PWAThemeColor, values[settings.PWAThemeColor])
	}
	if values[settings.SiteName] != "Carried Over" {
		t.Errorf("site.name = %v, want the imported value", values[settings.SiteName])
	}
}
