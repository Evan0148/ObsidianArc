package console

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/id"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

func TestProjectCommandsExecute(t *testing.T) {
	actor := user.User{ID: id.New(), Username: "tester", Role: user.RoleUser}

	type callRecord struct {
		Method string
		Path   string
		Body   any
	}
	var lastCall callRecord

	c := New(Options{Dispatch: func(_ context.Context, _ user.User, method, path string, body any) (Response, error) {
		lastCall = callRecord{Method: method, Path: path, Body: body}
		switch {
		case method == "GET" && path == "/api/projects":
			return Response{Status: 200, Body: []byte(`{"projects":[{"id":"01PROJ1","name":"Alpha","conversations":3,"created_at":1700000000000,"updated_at":1700001000000}],"max":64}`)}, nil
		case method == "GET" && path == "/api/projects/01PROJ1":
			return Response{Status: 200, Body: []byte(`{"id":"01PROJ1","name":"Alpha","instructions":"Be helpful and brief.","conversations":3,"created_at":1700000000000,"updated_at":1700001000000}`)}, nil
		case method == "POST" && path == "/api/projects":
			return Response{Status: 201, Body: []byte(`{"id":"01PROJ2","name":"Beta","instructions":"","conversations":0,"created_at":1700002000000,"updated_at":1700002000000}`)}, nil
		case method == "PATCH" && path == "/api/projects/01PROJ1":
			return Response{Status: 200, Body: []byte(`{"id":"01PROJ1","name":"AlphaUpdated","instructions":"Updated prompt","conversations":3,"created_at":1700000000000,"updated_at":1700003000000}`)}, nil
		case method == "DELETE" && path == "/api/projects/01PROJ1":
			return Response{Status: 204, Body: nil}, nil
		default:
			return Response{Status: 404, Body: []byte(`{"error":{"code":"not_found","message":"Not found."}}`)}, nil
		}
	}})

	newSession := func(lang string) *Session {
		return &Session{Actor: actor, Transport: "web", Lang: lang, Width: 100}
	}

	t.Run("project list", func(t *testing.T) {
		var out bytes.Buffer
		res := c.Execute(context.Background(), newSession("en"), &out, "project list")
		if !res.OK {
			t.Fatalf("project list failed: %v, output: %s", res, out.String())
		}
		if lastCall.Method != "GET" || lastCall.Path != "/api/projects" {
			t.Errorf("unexpected call: %v", lastCall)
		}
		if !strings.Contains(out.String(), "Alpha") || !strings.Contains(out.String(), "01PROJ1") {
			t.Errorf("output missing project Alpha: %s", out.String())
		}
	})

	t.Run("project show", func(t *testing.T) {
		var out bytes.Buffer
		res := c.Execute(context.Background(), newSession("en"), &out, "project show 01PROJ1")
		if !res.OK {
			t.Fatalf("project show failed: %v, output: %s", res, out.String())
		}
		if lastCall.Method != "GET" || lastCall.Path != "/api/projects/01PROJ1" {
			t.Errorf("unexpected call: %v", lastCall)
		}
		if !strings.Contains(out.String(), "Be helpful and brief.") {
			t.Errorf("output missing instructions: %s", out.String())
		}
	})

	t.Run("project create requires name", func(t *testing.T) {
		var out bytes.Buffer
		res := c.Execute(context.Background(), newSession("en"), &out, "project create")
		if res.OK {
			t.Fatal("project create without --name should fail")
		}
		if !strings.Contains(out.String(), "needs a name") {
			t.Errorf("expected error message about name, got: %s", out.String())
		}
	})

	t.Run("project create with name and instructions", func(t *testing.T) {
		var out bytes.Buffer
		res := c.Execute(context.Background(), newSession("en"), &out, `project create --name Beta --instructions "custom rules"`)
		if !res.OK {
			t.Fatalf("project create failed: %v, output: %s", res, out.String())
		}
		if lastCall.Method != "POST" || lastCall.Path != "/api/projects" {
			t.Errorf("unexpected call: %v", lastCall)
		}
		payload, _ := json.Marshal(lastCall.Body)
		if !strings.Contains(string(payload), "Beta") {
			t.Errorf("expected Beta in payload: %s", string(payload))
		}
	})

	t.Run("project edit requires flags", func(t *testing.T) {
		var out bytes.Buffer
		res := c.Execute(context.Background(), newSession("en"), &out, "project edit 01PROJ1")
		if res.OK {
			t.Fatal("project edit without flags should fail")
		}
		if !strings.Contains(out.String(), "nothing to change") {
			t.Errorf("expected nothing to change error, got: %s", out.String())
		}
	})

	t.Run("project edit modifies fields", func(t *testing.T) {
		var out bytes.Buffer
		res := c.Execute(context.Background(), newSession("en"), &out, `project edit 01PROJ1 --name "AlphaUpdated"`)
		if !res.OK {
			t.Fatalf("project edit failed: %v, output: %s", res, out.String())
		}
		if lastCall.Method != "PATCH" || lastCall.Path != "/api/projects/01PROJ1" {
			t.Errorf("unexpected call: %v", lastCall)
		}
		if !strings.Contains(out.String(), "AlphaUpdated") {
			t.Errorf("expected updated name in output: %s", out.String())
		}
	})

	t.Run("project delete requires confirmation", func(t *testing.T) {
		var out bytes.Buffer
		res := c.Execute(context.Background(), newSession("en"), &out, "project delete 01PROJ1")
		if res.OK {
			t.Fatal("project delete without --yes should fail")
		}
		if res.Code != "confirmation_required" {
			t.Errorf("expected confirmation_required, got: %s", res.Code)
		}
	})

	t.Run("project delete with confirmation succeeds", func(t *testing.T) {
		var out bytes.Buffer
		res := c.Execute(context.Background(), newSession("en"), &out, "project delete 01PROJ1 --yes")
		if !res.OK {
			t.Fatalf("project delete --yes failed: %v, output: %s", res, out.String())
		}
		if lastCall.Method != "DELETE" || lastCall.Path != "/api/projects/01PROJ1" {
			t.Errorf("unexpected call: %v", lastCall)
		}
		if !strings.Contains(out.String(), "deleted") {
			t.Errorf("expected deleted confirmation: %s", out.String())
		}
	})
}
