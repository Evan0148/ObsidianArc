package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The console's permission story, exercised as HTTP.
//
// internal/console proves the engine refuses what an actor may not run. What
// it cannot prove is that the endpoints underneath refuse it too — the engine
// there talks to a dispatcher a test wrote. These cases run the real thing:
// one process, the real admin handlers, a real delegated administrator.
//
// The property worth holding is narrow and important. The console is a second
// door onto the backoffice, so an account that may edit users and nothing
// else must be able to edit users through it and nothing else. A console that
// quietly widened a grant would be the most expensive bug this feature could
// have, and it would be invisible from the screen.

type doneVerdict struct {
	OK   bool   `json:"ok"`
	Code string `json:"code"`
	Exit bool   `json:"exit"`
}

// consoleRun executes one line and returns everything the terminal would have
// printed, plus the command's own verdict.
func consoleRun(t *testing.T, in *instance, as *session, line string) (string, doneVerdict) {
	t.Helper()
	response := in.do(http.MethodPost, "/api/admin/console/exec",
		map[string]any{"line": line, "cols": 100}, as)
	if response.Code != http.StatusOK {
		t.Fatalf("exec %q: %d %s", line, response.Code, response.Body.String())
	}
	return readConsoleStream(t, response)
}

func readConsoleStream(t *testing.T, response *httptest.ResponseRecorder) (string, doneVerdict) {
	t.Helper()
	var out strings.Builder
	var done doneVerdict
	seen := false

	for _, frame := range strings.Split(response.Body.String(), "\n\n") {
		var event, data string
		for _, line := range strings.Split(strings.TrimRight(frame, "\n"), "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			}
		}
		switch event {
		case "out":
			var payload struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				t.Fatalf("decode out frame %q: %v", data, err)
			}
			out.WriteString(payload.Text)
		case "done":
			if err := json.Unmarshal([]byte(data), &done); err != nil {
				t.Fatalf("decode done frame %q: %v", data, err)
			}
			seen = true
		}
	}
	if !seen {
		t.Fatalf("the stream never finished: %q", response.Body.String())
	}
	return out.String(), done
}

// delegate promotes an account to administrator holding exactly grants.
func delegate(t *testing.T, in *instance, founder *session, target *session, grants ...string) {
	t.Helper()
	response := in.do(http.MethodPatch, "/api/admin/users/"+target.userID, map[string]any{
		"role": "admin", "admin_permissions": grants,
	}, founder)
	if response.Code != http.StatusOK {
		t.Fatalf("delegate %v: %d %s", grants, response.Code, response.Body.String())
	}
}

func TestConsoleHoldsADelegatedAdministratorToTheirOwnGrants(t *testing.T) {
	in := newInstance(t)
	founder := in.register("founder", "a-good-password")
	limited := in.register("deskclerk", "another-password")
	delegate(t, in, founder, limited, "users")

	// The grant they hold: it works, and it really did reach the users
	// endpoint rather than being answered by the engine out of nowhere.
	output, done := consoleRun(t, in, limited, "user list")
	if !done.OK {
		t.Fatalf("user list as a users administrator failed: %s / %q", done.Code, output)
	}
	if !strings.Contains(output, "deskclerk") {
		t.Errorf("user list did not list the accounts:\n%s", output)
	}

	// Everything else they do not hold. Each of these is a different
	// permission string on the route behind it, so they are seven separate
	// chances for the console to have widened something.
	for _, line := range []string{
		"model list",
		"group list",
		"setting list",
		"provider list",
		"log list",
		"code list",
		"security events",
	} {
		output, done := consoleRun(t, in, limited, line)
		if done.OK {
			t.Errorf("%q succeeded for an administrator who only holds users:\n%s", line, output)
		}
	}
}

// Refusing to run the command is one half. Not advertising it is the other:
// a console that lists seventy commands and refuses sixty is worse than one
// that lists the ten that work.
func TestConsoleNeitherListsNorCompletesWhatTheActorCannotRun(t *testing.T) {
	in := newInstance(t)
	founder := in.register("founder", "a-good-password")
	limited := in.register("deskclerk", "another-password")
	delegate(t, in, founder, limited, "users")

	response := in.do(http.MethodGet, "/api/admin/console/spec", nil, limited)
	if response.Code != http.StatusOK {
		t.Fatalf("spec: %d %s", response.Code, response.Body.String())
	}
	var spec struct {
		You struct {
			Username    string   `json:"username"`
			Permissions []string `json:"permissions"`
		} `json:"you"`
		Commands []struct {
			Name       string `json:"name"`
			Permission string `json:"permission"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode spec: %v", err)
	}
	if spec.You.Username != "deskclerk" {
		t.Errorf("spec names %q", spec.You.Username)
	}

	named := map[string]bool{}
	for _, command := range spec.Commands {
		named[command.Name] = true
	}
	if !named["user list"] {
		t.Error("the spec hides user list from an administrator who holds users")
	}
	for _, hidden := range []string{"model list", "setting set", "group delete", "provider create"} {
		if named[hidden] {
			t.Errorf("the spec offers %q to an administrator who cannot run it", hidden)
		}
	}

	// The same rule at the prompt: completing "mod" must not reveal the model
	// commands to someone who cannot run them.
	response = in.do(http.MethodPost, "/api/admin/console/complete",
		map[string]any{"line": "mod", "pos": 3}, limited)
	if response.Code != http.StatusOK {
		t.Fatalf("complete: %d %s", response.Code, response.Body.String())
	}
	var completion struct {
		Items []struct {
			Value string `json:"value"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &completion); err != nil {
		t.Fatalf("decode completion: %v", err)
	}
	for _, item := range completion.Items {
		if strings.HasPrefix(item.Value, "model") {
			t.Errorf("completion offered %q to an administrator who cannot run it", item.Value)
		}
	}
}

// The other direction: a super administrator is not held to a narrower list
// than the backoffice gives them, or the console would be a downgrade.
func TestConsoleGivesASuperAdministratorTheWholeSurface(t *testing.T) {
	in := newInstance(t)
	founder := in.register("founder", "a-good-password")

	for _, line := range []string{"user list", "model list", "group list", "setting list", "dash"} {
		output, done := consoleRun(t, in, founder, line)
		if !done.OK {
			t.Errorf("%q failed for a super administrator: %s / %q", line, done.Code, output)
		}
	}
}

// A regular account cannot reach the console at all — the section is not
// permission-gated the way the others are, so this is the only thing standing
// between it and everyone.
func TestConsoleIsClosedToAccountsThatAreNotAdministrators(t *testing.T) {
	in := newInstance(t)
	in.register("founder", "a-good-password")
	visitor := in.register("visitor", "another-password")

	for _, attempt := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/admin/console/spec", nil},
		{http.MethodPost, "/api/admin/console/exec", map[string]any{"line": "user list"}},
		{http.MethodPost, "/api/admin/console/complete", map[string]any{"line": "u", "pos": 1}},
	} {
		if code := in.do(attempt.method, attempt.path, attempt.body, visitor).Code; code != http.StatusForbidden {
			t.Errorf("%s %s as a regular account = %d, want 403", attempt.method, attempt.path, code)
		}
	}
}

// Destructive commands are the ones where a slip is not recoverable, so the
// console asks for the word rather than reading the intent.
func TestConsoleRefusesADestructiveCommandWithoutConsent(t *testing.T) {
	in := newInstance(t)
	founder := in.register("founder", "a-good-password")
	victim := in.register("victim", "another-password")

	output, done := consoleRun(t, in, founder, "user delete "+victim.userID)
	if done.OK {
		t.Fatalf("user delete ran without --yes:\n%s", output)
	}

	// And the account is still there, which is the part that matters.
	response := in.do(http.MethodGet, "/api/admin/users/"+victim.userID, nil, founder)
	if response.Code != http.StatusOK {
		t.Fatalf("the account was deleted anyway: %d %s", response.Code, response.Body.String())
	}
}
