package server

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestAdminUsageRPMAndCustomTime(t *testing.T) {
	in := newInstance(t)
	admin := in.register("usage_admin", "password123")

	// 1. Check GET /api/admin/usage/rpm
	rpmRes := in.do(http.MethodGet, "/api/admin/usage/rpm", nil, admin)
	if rpmRes.Code != http.StatusOK {
		t.Fatalf("expected 200 from /api/admin/usage/rpm, got %d: %s", rpmRes.Code, rpmRes.Body.String())
	}
	var rpmPayload struct {
		RPM int64 `json:"rpm"`
	}
	rpmPayload = decode[struct {
		RPM int64 `json:"rpm"`
	}](t, rpmRes)
	if rpmPayload.RPM != 0 {
		t.Errorf("expected rpm = 0, got %d", rpmPayload.RPM)
	}

	// 2. Check GET /api/admin/usage returns current_rpm
	usageRes := in.do(http.MethodGet, "/api/admin/usage", nil, admin)
	if usageRes.Code != http.StatusOK {
		t.Fatalf("expected 200 from /api/admin/usage, got %d: %s", usageRes.Code, usageRes.Body.String())
	}
	var usagePayload struct {
		CurrentRPM int64 `json:"current_rpm"`
		BucketMS   int64 `json:"bucket_ms"`
	}
	usagePayload = decode[struct {
		CurrentRPM int64 `json:"current_rpm"`
		BucketMS   int64 `json:"bucket_ms"`
	}](t, usageRes)
	if usagePayload.CurrentRPM != 0 {
		t.Errorf("expected current_rpm = 0, got %d", usagePayload.CurrentRPM)
	}

	// 3. Check custom range with since and until
	since := time.Now().Add(-2 * time.Hour).UnixMilli()
	until := time.Now().Add(-1 * time.Hour).UnixMilli()
	customRes := in.do(http.MethodGet, "/api/admin/usage?since="+strconv.FormatInt(since, 10)+"&until="+strconv.FormatInt(until, 10), nil, admin)
	if customRes.Code != http.StatusOK {
		t.Fatalf("expected 200 from /api/admin/usage with custom range, got %d: %s", customRes.Code, customRes.Body.String())
	}
}
