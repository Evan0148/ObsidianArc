package server

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/totp"
)

// What reaches an account's bell from the wiring: the administrator's changes
// to it, and the sign-ins it did not expect. Each producer is a line in a
// handler or a hook in server.go, so only the assembled server shows that the
// line is there and says what the browser words.

type notice struct {
	Kind   string         `json:"kind"`
	Params map[string]any `json:"params"`
	Link   string         `json:"link"`
}

func (in *instance) notices(as *session) []notice {
	in.t.Helper()
	response := in.do(http.MethodGet, "/api/notifications", nil, as)
	if response.Code != http.StatusOK {
		in.t.Fatalf("notifications: %d %s", response.Code, response.Body.String())
	}
	return decode[struct {
		Notifications []notice `json:"notifications"`
	}](in.t, response).Notifications
}

func kinds(list []notice) []string {
	out := make([]string, 0, len(list))
	for _, n := range list {
		out = append(out, n.Kind)
	}
	return out
}

func find(list []notice, kind string) []notice {
	var out []notice
	for _, n := range list {
		if n.Kind == kind {
			out = append(out, n)
		}
	}
	return out
}

func TestAnAdministratorsChangesReachTheAccountsBell(t *testing.T) {
	in := newInstance(t)
	founder := in.register("founder", "a-good-password")
	member := in.register("member", "another-password")

	grant := in.do(http.MethodPost, "/api/admin/users/"+member.userID+"/cards",
		map[string]any{"cards": 2, "card_days": 7}, founder)
	if grant.Code != http.StatusCreated {
		t.Fatalf("grant: %d %s", grant.Code, grant.Body.String())
	}

	rename := map[string]any{"nickname": "Someone"}
	if response := in.do(http.MethodPatch, "/api/admin/users/"+member.userID, rename, founder); response.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", response.Code, response.Body.String())
	}
	// The backoffice saves the whole form every time. The same values again
	// changed nothing, and a notice saying otherwise would be noise.
	if response := in.do(http.MethodPatch, "/api/admin/users/"+member.userID, rename, founder); response.Code != http.StatusOK {
		t.Fatalf("resave: %d %s", response.Code, response.Body.String())
	}
	// An administrator's own edit is not news to them.
	if response := in.do(http.MethodPatch, "/api/admin/users/"+founder.userID,
		map[string]any{"nickname": "Boss"}, founder); response.Code != http.StatusOK {
		t.Fatalf("self edit: %d %s", response.Code, response.Body.String())
	}
	if got := find(in.notices(founder), "account_changed"); len(got) != 0 {
		t.Fatalf("an administrator was told about their own edit: %+v", got)
	}

	list := in.notices(member)
	cards := find(list, "cards_granted")
	if len(cards) != 1 || cards[0].Params["count"] != float64(2) || cards[0].Link != "/usage" {
		t.Fatalf("cards_granted: %+v (all: %v)", cards, kinds(list))
	}
	changed := find(list, "account_changed")
	if len(changed) != 1 {
		t.Fatalf("account_changed %d times, want once: %v", len(changed), kinds(list))
	}
	if what, _ := changed[0].Params["what"].([]any); len(what) != 1 || what[0] != "profile" {
		t.Fatalf("account_changed names %v, want [profile]", changed[0].Params["what"])
	}

	// A password reset ends every session, so it is read at the next sign-in.
	reset := in.do(http.MethodPost, "/api/admin/users/"+member.userID+"/password",
		map[string]string{"new_password": "a-third-password"}, founder)
	if reset.Code != http.StatusNoContent {
		t.Fatalf("reset password: %d %s", reset.Code, reset.Body.String())
	}
	back := in.login("member", "a-third-password")
	var password bool
	for _, n := range find(in.notices(back), "account_changed") {
		if what, _ := n.Params["what"].([]any); len(what) == 1 && what[0] == "password" {
			password = true
		}
	}
	if !password {
		t.Fatalf("no notice of the password reset: %v", kinds(in.notices(back)))
	}
}

func TestASignInFromANewDeviceIsLoggedAndTold(t *testing.T) {
	in := newInstance(t)
	founder := in.register("founder", "a-good-password")
	if got := find(in.notices(founder), "new_device_login"); len(got) != 0 {
		t.Fatalf("the first device an account ever used was called new: %+v", got)
	}

	// No device cookie rides along with the test client, so this sign-in is
	// from a device the account has not seen.
	again := in.login("founder", "a-good-password")
	fresh := find(in.notices(again), "new_device_login")
	if len(fresh) != 1 || fresh[0].Link != "/settings?tab=security" {
		t.Fatalf("new_device_login after a second device: %+v", fresh)
	}
	if _, ok := fresh[0].Params["ua"]; !ok {
		t.Fatalf("the notice does not carry the device to describe: %+v", fresh[0].Params)
	}
	events := in.do(http.MethodGet, "/api/admin/security/events?event=new_device", nil, again)
	if events.Code != http.StatusOK || !strings.Contains(events.Body.String(), `"event":"new_device"`) {
		t.Fatalf("the security log has no new-device entry: %d %s", events.Code, events.Body.String())
	}

	// Two-step sign-in issues its full session in a different handler; it is
	// a sign-in all the same, and switching the factor on is told too.
	secret, used := in.enrol(again)
	if got := find(in.notices(again), "two_factor_changed"); len(got) != 1 || got[0].Params["kind"] != "enabled" {
		t.Fatalf("two_factor_changed after enrolling: %+v", got)
	}
	login := in.do(http.MethodPost, "/api/auth/login",
		map[string]string{"identifier": "founder", "password": "a-good-password"}, nil)
	half := sessionFrom(t, login)
	code, _ := totp.Code(secret, used+1)
	done := in.do(http.MethodPost, "/api/auth/two-factor", map[string]string{"code": code}, half)
	if done.Code != http.StatusOK {
		t.Fatalf("complete: %d %s", done.Code, done.Body.String())
	}
	var device bool
	for _, cookie := range done.Result().Cookies() {
		device = device || (strings.HasSuffix(cookie.Name, "_device") && cookie.Value != "")
	}
	if !device {
		t.Fatal("finishing a two-step sign-in did not give the browser a device")
	}
	if got := find(in.notices(sessionFrom(t, done)), "new_device_login"); len(got) != 2 {
		t.Fatalf("new_device_login %d times after a third device, want 2: %v",
			len(got), kinds(in.notices(sessionFrom(t, done))))
	}
	if slices.Contains(kinds(in.notices(sessionFrom(t, done))), "") {
		t.Fatal("a notice with no kind")
	}
}
