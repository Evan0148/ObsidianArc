package console

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/id"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

func TestSelfInviteCommands(t *testing.T) {
	actor := user.User{ID: id.New(), Username: "alice", Role: user.RoleUser}

	type callRecord struct {
		Method string
		Path   string
	}
	var lastCall callRecord
	var regenerated bool

	c := New(Options{Dispatch: func(_ context.Context, _ user.User, method, path string, _ any) (Response, error) {
		lastCall = callRecord{Method: method, Path: path}
		switch {
		case method == "GET" && path == "/api/profile/invites":
			code := "AB12CD34"
			if regenerated {
				code = "ZZ99YY88"
			}
			return Response{Status: 200, Body: []byte(`{"enabled":true,"code":"` + code + `","limit":10,"used":1,
				"reward_cards":2,"reward_card_days":30,"invitees":[
					{"nickname":"Bob","username":"bob","created_at":1700000000000,"rewarded":true,"reward_skipped":""},
					{"nickname":"","username":"carl","created_at":1700001000000,"rewarded":false,"reward_skipped":"limit"}
				]}`)}, nil
		case method == "POST" && path == "/api/profile/invites/regenerate":
			regenerated = true
			return Response{Status: 200, Body: []byte(`{"enabled":true,"code":"ZZ99YY88","limit":10,"used":0,
				"reward_cards":2,"reward_card_days":30,"invitees":[]}`)}, nil
		default:
			return Response{Status: 404, Body: []byte(`{"error":{"code":"not_found","message":"Not found."}}`)}, nil
		}
	}})

	run := func(line string) (Result, string) {
		var out bytes.Buffer
		s := &Session{Actor: actor, Transport: "web", Lang: "en", Width: 120}
		return c.Execute(context.Background(), s, &out, line), out.String()
	}

	t.Run("me invite shows the code hyphenated and the invitee table", func(t *testing.T) {
		result, out := run("me invite")
		if !result.OK {
			t.Fatalf("me invite failed: %s", out)
		}
		if lastCall.Method != "GET" || lastCall.Path != "/api/profile/invites" {
			t.Errorf("unexpected call: %v", lastCall)
		}
		if !strings.Contains(out, "AB12-CD34") {
			t.Errorf("expected the hyphenated code:\n%s", out)
		}
		if !strings.Contains(out, "bob") || !strings.Contains(out, "carl") {
			t.Errorf("expected both invitees listed:\n%s", out)
		}
		if !strings.Contains(out, "limit") {
			t.Errorf("expected carl's skip reason shown:\n%s", out)
		}
	})

	t.Run("regenerate refuses without --yes", func(t *testing.T) {
		result, _ := run("me invite regenerate")
		if result.OK {
			t.Fatal("me invite regenerate without --yes should be refused")
		}
		if result.Code != "confirmation_required" {
			t.Errorf("want confirmation_required, got %q", result.Code)
		}
		if regenerated {
			t.Fatal("the API must not be called before confirmation")
		}
	})

	t.Run("regenerate replaces the code", func(t *testing.T) {
		result, out := run("me invite regenerate --yes")
		if !result.OK {
			t.Fatalf("me invite regenerate failed: %s", out)
		}
		if lastCall.Method != "POST" || lastCall.Path != "/api/profile/invites/regenerate" {
			t.Errorf("unexpected call: %v", lastCall)
		}
		if !strings.Contains(out, "ZZ99-YY88") {
			t.Errorf("expected the new code hyphenated:\n%s", out)
		}
	})

	t.Run("a disabled personal code shows as a dash, not an error", func(t *testing.T) {
		disabled := New(Options{Dispatch: func(_ context.Context, _ user.User, method, path string, _ any) (Response, error) {
			return Response{Status: 200, Body: []byte(`{"enabled":false,"code":"","limit":10,"used":0,
				"reward_cards":0,"reward_card_days":30,"invitees":[]}`)}, nil
		}})
		var out bytes.Buffer
		s := &Session{Actor: actor, Transport: "web", Lang: "en", Width: 120}
		result := disabled.Execute(context.Background(), s, &out, "me invite")
		if !result.OK {
			t.Fatalf("me invite failed: %s", out.String())
		}
		codeLine := ""
		for _, line := range strings.Split(out.String(), "\n") {
			if strings.HasPrefix(line, "code:") {
				codeLine = line
				break
			}
		}
		if !strings.HasSuffix(strings.TrimSpace(codeLine), "-") {
			t.Errorf("expected the code field to print as a dash when disabled, got %q:\n%s", codeLine, out.String())
		}
	})
}
