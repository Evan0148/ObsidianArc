package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSiteLogoLifecycle(t *testing.T) {
	in := newInstance(t)
	admin := in.register("admin", "a-strong-password")
	regular := in.register("regular", "another-strong-password")

	tinyPNG := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	// 1. Initially GET /api/site returns empty logo_url
	siteRes := in.do(http.MethodGet, "/api/site", nil, nil)
	if siteRes.Code != http.StatusOK {
		t.Fatalf("GET /api/site: %d", siteRes.Code)
	}
	var siteData struct {
		LogoURL string `json:"logo_url"`
	}
	if err := json.Unmarshal(siteRes.Body.Bytes(), &siteData); err != nil {
		t.Fatalf("unmarshal /api/site: %v", err)
	}
	if siteData.LogoURL != "" {
		t.Fatalf("expected empty logo_url initially, got: %s", siteData.LogoURL)
	}

	// 2. Anonymous / regular user cannot upload
	anonRes := in.do(http.MethodPut, "/api/admin/logo", map[string]string{
		"mime": "image/png",
		"data": tinyPNG,
	}, nil)
	if anonRes.Code != http.StatusUnauthorized {
		t.Fatalf("anon upload: %d, want 401", anonRes.Code)
	}

	userRes := in.do(http.MethodPut, "/api/admin/logo", map[string]string{
		"mime": "image/png",
		"data": tinyPNG,
	}, regular)
	if userRes.Code != http.StatusForbidden {
		t.Fatalf("regular upload: %d, want 403", userRes.Code)
	}

	// 3. Admin upload invalid format -> 400
	badRes := in.do(http.MethodPut, "/api/admin/logo", map[string]string{
		"mime": "text/plain",
		"data": "aGVsbG8gd29ybGQ=",
	}, admin)
	if badRes.Code != http.StatusBadRequest {
		t.Fatalf("bad logo upload: %d, want 400", badRes.Code)
	}

	// 4. Admin upload PNG -> 200
	putRes := in.do(http.MethodPut, "/api/admin/logo", map[string]string{
		"mime": "image/png",
		"data": tinyPNG,
	}, admin)
	if putRes.Code != http.StatusOK {
		t.Fatalf("admin logo upload: %d %s, want 200", putRes.Code, putRes.Body.String())
	}
	var putPayload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(putRes.Body.Bytes(), &putPayload); err != nil {
		t.Fatalf("unmarshal upload response: %v", err)
	}
	if !strings.HasPrefix(putPayload.URL, "/api/site/logo?v=") {
		t.Fatalf("expected url to start with /api/site/logo?v=, got %s", putPayload.URL)
	}

	// 5. GET /api/site now returns the logo_url
	siteRes = in.do(http.MethodGet, "/api/site", nil, nil)
	_ = json.Unmarshal(siteRes.Body.Bytes(), &siteData)
	if siteData.LogoURL != putPayload.URL {
		t.Fatalf("GET /api/site logo_url = %q, want %q", siteData.LogoURL, putPayload.URL)
	}

	// 6. GET /manifest.webmanifest now returns the logo as icon
	manifestRes := in.do(http.MethodGet, "/manifest.webmanifest", nil, nil)
	if manifestRes.Code != http.StatusOK {
		t.Fatalf("GET /manifest.webmanifest: %d", manifestRes.Code)
	}
	var manifestData struct {
		Icons []struct {
			Src string `json:"src"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(manifestRes.Body.Bytes(), &manifestData); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(manifestData.Icons) == 0 || manifestData.Icons[0].Src != putPayload.URL {
		t.Fatalf("manifest icon = %v, want %s", manifestData.Icons, putPayload.URL)
	}

	// 7. GET /api/site/logo serves bytes and caching headers
	getRes := in.do(http.MethodGet, "/api/site/logo", nil, nil)
	if getRes.Code != http.StatusOK {
		t.Fatalf("GET /api/site/logo: %d", getRes.Code)
	}
	if ct := getRes.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type = %s, want image/png", ct)
	}
	etag := getRes.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("expected ETag header on /api/site/logo")
	}

	// 8. ETag 304 Not Modified
	reqEtag := httptest.NewRequest(http.MethodGet, "/api/site/logo", nil)
	reqEtag.Header.Set("If-None-Match", etag)
	wEtag := httptest.NewRecorder()
	in.handler.ServeHTTP(wEtag, reqEtag)
	if wEtag.Code != http.StatusNotModified {
		t.Fatalf("ETag caching: %d, want 304", wEtag.Code)
	}

	// 9. Admin delete logo
	delRes := in.do(http.MethodDelete, "/api/admin/logo", nil, admin)
	if delRes.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/admin/logo: %d, want 204", delRes.Code)
	}

	// 10. GET /api/site/logo returns 404
	getDeletedRes := in.do(http.MethodGet, "/api/site/logo", nil, nil)
	if getDeletedRes.Code != http.StatusNotFound {
		t.Fatalf("GET /api/site/logo after deletion: %d, want 404", getDeletedRes.Code)
	}

	// 11. GET /api/site returns empty logo_url
	siteRes = in.do(http.MethodGet, "/api/site", nil, nil)
	_ = json.Unmarshal(siteRes.Body.Bytes(), &siteData)
	if siteData.LogoURL != "" {
		t.Fatalf("logo_url after deletion = %s, want empty", siteData.LogoURL)
	}
}
