package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/totp"
)

// Two-step sign-in through the assembled server: the wiring is where the
// enrolment gate, the backoffice's second lock and the pending session meet,
// and none of the three is visible from inside the package that defines it.

func sessionFrom(t *testing.T, response *httptest.ResponseRecorder) *session {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "obsidian_session" && cookie.Value != "" {
			return &session{cookie: cookie}
		}
	}
	t.Fatalf("no session cookie in %d %s", response.Code, response.Body.String())
	return nil
}

// enrol switches the second step on for a signed-in account over HTTP and
// returns the secret and the step its confirming code spent.
func (in *instance) enrol(as *session) (string, int64) {
	in.t.Helper()
	setup := in.do(http.MethodPost, "/api/profile/two-factor/setup", nil, as)
	if setup.Code != http.StatusOK {
		in.t.Fatalf("setup: %d %s", setup.Code, setup.Body.String())
	}
	secret := decode[struct {
		Secret string `json:"secret"`
		QR     struct {
			Size int    `json:"size"`
			Path string `json:"path"`
		} `json:"qr"`
	}](in.t, setup)
	if secret.QR.Size == 0 || secret.QR.Path == "" {
		in.t.Fatalf("setup drew no QR code: %s", setup.Body.String())
	}
	step := totp.Step(time.Now())
	code, _ := totp.Code(secret.Secret, step)
	enable := in.do(http.MethodPost, "/api/profile/two-factor/enable", map[string]string{"code": code}, as)
	if enable.Code != http.StatusOK {
		in.t.Fatalf("enable: %d %s", enable.Code, enable.Body.String())
	}
	codes := decode[struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}](in.t, enable)
	if len(codes.RecoveryCodes) != totp.RecoveryCount {
		in.t.Fatalf("enable returned %d recovery codes", len(codes.RecoveryCodes))
	}
	return secret.Secret, step
}

func TestSigningInWithASecondStepOverHTTP(t *testing.T) {
	in := newInstance(t)
	founder := in.register("founder", "a-good-password")
	secret, used := in.enrol(founder)

	login := in.do(http.MethodPost, "/api/auth/login",
		map[string]string{"identifier": "founder", "password": "a-good-password"}, nil)
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), `"two_factor":true`) ||
		strings.Contains(login.Body.String(), `"user"`) {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	half := sessionFrom(t, login)

	// Halfway is not signed in, and says which half is missing.
	me := in.do(http.MethodGet, "/api/auth/me", nil, half)
	if me.Code != http.StatusUnauthorized || !strings.Contains(me.Body.String(), "two_factor_pending") {
		t.Fatalf("me while pending: %d %s", me.Code, me.Body.String())
	}
	if code := in.do(http.MethodGet, "/api/conversations", nil, half).Code; code != http.StatusUnauthorized {
		t.Fatalf("the API answered a pending session: %d", code)
	}

	wrong := in.do(http.MethodPost, "/api/auth/two-factor", map[string]string{"code": "000000"}, half)
	if wrong.Code != http.StatusBadRequest || !strings.Contains(wrong.Body.String(), "two_factor_code") {
		t.Fatalf("a wrong code: %d %s", wrong.Code, wrong.Body.String())
	}

	code, _ := totp.Code(secret, used+1)
	done := in.do(http.MethodPost, "/api/auth/two-factor", map[string]string{"code": code}, half)
	if done.Code != http.StatusOK || !strings.Contains(done.Body.String(), `"username":"founder"`) {
		t.Fatalf("complete: %d %s", done.Code, done.Body.String())
	}
	full := sessionFrom(t, done)
	if full.cookie.Value == half.cookie.Value {
		t.Fatal("the pending token was promoted instead of replaced")
	}
	if code := in.do(http.MethodGet, "/api/auth/me", nil, full).Code; code != http.StatusOK {
		t.Fatalf("me after completing: %d", code)
	}
}

func TestTheTwoStepPolicyThroughTheWiring(t *testing.T) {
	in := newInstance(t)
	founder := in.register("founder", "a-good-password")
	member := in.register("member", "another-password")

	// Requiring it without having it would shut the founder out of the very
	// screen that could undo it.
	policy := map[string]string{"security.two_factor_policy": "backoffice"}
	if response := in.do(http.MethodPut, "/api/admin/settings", policy, founder); response.Code != http.StatusForbidden ||
		!strings.Contains(response.Body.String(), "two_factor_self") {
		t.Fatalf("policy without one's own second step: %d %s", response.Code, response.Body.String())
	}
	in.enrol(founder)
	if response := in.do(http.MethodPut, "/api/admin/settings", policy, founder); response.Code != http.StatusOK {
		t.Fatalf("policy after enrolling: %d %s", response.Code, response.Body.String())
	}

	// A second administrator without it: the chat works, the backoffice does
	// not — and the console dispatches into the same guarded routes.
	promote := in.do(http.MethodPatch, "/api/admin/users/"+member.userID,
		map[string]any{"role": "admin", "admin_permissions": []string{"dashboard"}}, founder)
	if promote.Code != http.StatusOK {
		t.Fatalf("promote: %d %s", promote.Code, promote.Body.String())
	}
	if code := in.do(http.MethodGet, "/api/conversations", nil, member).Code; code != http.StatusOK {
		t.Fatalf("the chat refused an administrator under the backoffice policy: %d", code)
	}
	backoffice := in.do(http.MethodGet, "/api/admin/dashboard", nil, member)
	if backoffice.Code != http.StatusForbidden || !strings.Contains(backoffice.Body.String(), "two_factor_backoffice") {
		t.Fatalf("backoffice without a second step: %d %s", backoffice.Code, backoffice.Body.String())
	}
	me := in.do(http.MethodGet, "/api/auth/me", nil, member)
	if !strings.Contains(me.Body.String(), `"two_factor_backoffice":true`) ||
		!strings.Contains(me.Body.String(), `"two_factor_enrol":false`) {
		t.Fatalf("the account payload does not say what the policy means: %s", me.Body.String())
	}

	// Everyone: the member is held at the door until they enrol.
	in.do(http.MethodPut, "/api/admin/settings", map[string]string{"security.two_factor_policy": "everyone"}, founder)
	held := in.do(http.MethodGet, "/api/conversations", nil, member)
	if held.Code != http.StatusForbidden || !strings.Contains(held.Body.String(), "two_factor_enrolment_required") {
		t.Fatalf("an unenrolled account under the everyone policy: %d %s", held.Code, held.Body.String())
	}
	if !strings.Contains(in.do(http.MethodGet, "/api/auth/me", nil, member).Body.String(), `"two_factor_enrol":true`) {
		t.Fatal("the account payload does not say it must enrol")
	}
	in.enrol(member)
	if code := in.do(http.MethodGet, "/api/conversations", nil, member).Code; code != http.StatusOK {
		t.Fatalf("still held after enrolling: %d", code)
	}
	if code := in.do(http.MethodGet, "/api/admin/dashboard", nil, member).Code; code != http.StatusOK {
		t.Fatalf("the backoffice still refuses after enrolling: %d", code)
	}

	// Switching off is refused while the policy covers the account.
	off := in.do(http.MethodPost, "/api/profile/two-factor/disable", map[string]string{"code": "000000"}, member)
	if off.Code != http.StatusForbidden || !strings.Contains(off.Body.String(), "two_factor_mandatory") {
		t.Fatalf("switching off under the policy: %d %s", off.Code, off.Body.String())
	}

	// The operator's reset, and what the adoption screen reads.
	adoption := in.do(http.MethodGet, "/api/admin/security/two-factor", nil, founder)
	if adoption.Code != http.StatusOK || !strings.Contains(adoption.Body.String(), `"enabled":2`) {
		t.Fatalf("adoption: %d %s", adoption.Code, adoption.Body.String())
	}
	reset := in.do(http.MethodDelete, "/api/admin/users/"+member.userID+"/two-factor", nil, founder)
	if reset.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", reset.Code, reset.Body.String())
	}
	if code := in.do(http.MethodGet, "/api/conversations", nil, member).Code; code != http.StatusForbidden {
		t.Fatalf("after a reset under the everyone policy the account should enrol again: %d", code)
	}
	events := in.do(http.MethodGet, "/api/admin/security/events?event=two_factor", nil, founder)
	if !strings.Contains(events.Body.String(), `"decision":"reset"`) ||
		!strings.Contains(events.Body.String(), `"decision":"enabled"`) {
		t.Fatalf("the security log is missing two-step entries: %s", events.Body.String())
	}
}
