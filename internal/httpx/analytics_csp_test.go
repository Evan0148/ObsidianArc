package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The analytics tag is a third party loading a script and posting what it
// recorded. The policy opens for it only while an instance has configured one,
// which is the whole reason this is a setting rather than a line in the shell:
// an instance that does not use it must not be carrying the hole.
func TestClarityWideningFollowsTheSetting(t *testing.T) {
	policyFor := func(challenge, analytics bool) string {
		handler := SecurityHeaders(false, nil,
			func() bool { return challenge },
			func() bool { return analytics },
		)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		return recorder.Header().Get("Content-Security-Policy")
	}

	off := policyFor(false, false)
	if strings.Contains(off, "clarity") || strings.Contains(off, "bing") {
		t.Fatalf("unconfigured instance carries the analytics exception: %s", off)
	}
	if !strings.Contains(off, "script-src 'self'") || !strings.Contains(off, "connect-src 'self'") {
		t.Fatalf("base policy lost its shape: %s", off)
	}

	on := policyFor(false, true)
	for _, want := range []string{"https://www.clarity.ms", "https://*.clarity.ms", "https://c.bing.com"} {
		if !strings.Contains(on, want) {
			t.Errorf("configured instance is missing %q: %s", want, on)
		}
	}
	// The script origin belongs in script-src and the ingest origins in
	// connect-src; swapping them silently blocks either the tag or its uploads.
	script := section(on, "script-src")
	connect := section(on, "connect-src")
	if !strings.Contains(script, "https://www.clarity.ms") {
		t.Errorf("script-src missing the tag origin: %s", script)
	}
	if !strings.Contains(connect, "https://*.clarity.ms") {
		t.Errorf("connect-src missing the ingest origin: %s", connect)
	}

	// The two switches are independent: turning analytics on must not drop the
	// challenge's frame-src, and turning the challenge on must not imply
	// analytics.
	both := policyFor(true, true)
	if !strings.Contains(both, "frame-src https://challenges.cloudflare.com") {
		t.Errorf("challenge exception lost when analytics is on: %s", both)
	}
	challengeOnly := policyFor(true, false)
	if strings.Contains(challengeOnly, "clarity") {
		t.Errorf("challenge alone dragged in the analytics exception: %s", challengeOnly)
	}
}

func section(policy, directive string) string {
	for _, part := range strings.Split(policy, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, directive+" ") {
			return part
		}
	}
	return ""
}
