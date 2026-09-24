package console

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/id"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// The commands that closed the gap between the account's screens and the
// terminal: its own feedback threads, its image history, and rewriting a
// message. Each case checks the request that reaches the API, not just the
// output, because a command that prints something plausible after calling the
// wrong endpoint is the failure these would otherwise hide.
func TestAccountCommandsReachTheirOwnEndpoints(t *testing.T) {
	actor := user.User{ID: id.New(), Username: "tester", Role: user.RoleUser}
	messageID := id.New()

	type call struct {
		method, path string
		body         any
	}
	var calls []call
	c := New(Options{Dispatch: func(_ context.Context, _ user.User, method, path string, body any) (Response, error) {
		calls = append(calls, call{method, path, body})
		switch {
		case method == "GET" && path == "/api/feedback":
			return Response{Status: 200, Body: []byte(`{"feedback":[{"id":"01FB","status":"open","kind":"bug","title":"Upload fails","replies":2,"author_unread":true,"updated_at":1700000000000}],"remaining":7,"max_per_day":10}`)}, nil
		case method == "GET" && path == "/api/feedback/01FB":
			return Response{Status: 200, Body: []byte(`{"feedback":{"id":"01FB","title":"Upload fails","status":"open","kind":"bug","priority":"medium","body":"413 over 8 MB","created_at":1700000000000},"replies":[{"from_staff":true,"body":"Looking into it","created_at":1700000100000},{"from_staff":false,"body":"Thanks","created_at":1700000200000}]}`)}, nil
		case method == "POST" && path == "/api/feedback/01FB/replies":
			return Response{Status: 201, Body: []byte(`{"id":"01RP"}`)}, nil
		case method == "GET" && path == "/api/feedback/unread":
			return Response{Status: 200, Body: []byte(`{"unread":3}`)}, nil
		case method == "GET" && strings.HasPrefix(path, "/api/images/generations?"):
			return Response{Status: 200, Body: []byte(`{"generations":[{"id":"01IMG","size":"1024x1024","prompt":"a lighthouse","url":"/api/attachments/01AT","created_at":1700000000000}]}`)}, nil
		case method == "DELETE" && path == "/api/images/generations/01IMG":
			return Response{Status: 204}, nil
		case method == "GET" && path == "/api/conversations/01CONV":
			return Response{Status: 200, Body: []byte(`{"conversation":{"id":"01CONV"},"messages":[{"id":"` + messageID + `","seq":1,"role":"user","content":"old"}]}`)}, nil
		case method == "PATCH" && path == "/api/conversations/01CONV/messages/"+messageID:
			return Response{Status: 200, Body: []byte(`{"message":{}}`)}, nil
		}
		return Response{Status: 404, Body: []byte(`{"error":{"code":"not_found","message":"Not found."}}`)}, nil
	}})
	run := func(line string) (Result, string) {
		calls = nil
		var out bytes.Buffer
		s := &Session{Actor: actor, Transport: "web", Lang: "en", Width: 120}
		return c.Execute(context.Background(), s, &out, line), out.String()
	}

	t.Run("feedback mine lists the reports and what is left today", func(t *testing.T) {
		result, out := run("feedback mine")
		if !result.OK || !strings.Contains(out, "Upload fails") || !strings.Contains(out, "7 of 10") {
			t.Fatalf("result %+v, output:\n%s", result, out)
		}
	})

	t.Run("feedback thread shows the whole conversation", func(t *testing.T) {
		result, out := run("feedback thread 01FB")
		if !result.OK || !strings.Contains(out, "413 over 8 MB") || !strings.Contains(out, "Looking into it") || !strings.Contains(out, "— you") {
			t.Fatalf("result %+v, output:\n%s", result, out)
		}
	})

	t.Run("feedback answer speaks on the author's own thread", func(t *testing.T) {
		result, out := run(`feedback answer 01FB --body "Still broken"`)
		if !result.OK || len(calls) != 1 || calls[0].path != "/api/feedback/01FB/replies" {
			t.Fatalf("result %+v, calls %+v, output:\n%s", result, calls, out)
		}
		if body, _ := calls[0].body.(map[string]any); body["body"] != "Still broken" {
			t.Errorf("sent %+v", calls[0].body)
		}
	})

	t.Run("feedback unread counts", func(t *testing.T) {
		if result, out := run("feedback unread"); !result.OK || !strings.Contains(out, "3") {
			t.Fatalf("result %+v, output:\n%s", result, out)
		}
	})

	t.Run("image list and delete", func(t *testing.T) {
		if result, out := run("image list --limit 5"); !result.OK || !strings.Contains(out, "a lighthouse") || !strings.Contains(calls[0].path, "limit=5") {
			t.Fatalf("list: result %+v, calls %+v, output:\n%s", result, calls, out)
		}
		if result, _ := run("image delete 01IMG"); result.OK || len(calls) != 0 {
			t.Fatalf("image delete ran without --yes: %+v, calls %+v", result, calls)
		}
		if result, out := run("image delete 01IMG --yes"); !result.OK || calls[0].method != "DELETE" {
			t.Fatalf("delete: result %+v, calls %+v, output:\n%s", result, calls, out)
		}
	})

	t.Run("chat edit finds the message by its number", func(t *testing.T) {
		result, out := run(`chat edit 01CONV 1 --content "new text"`)
		if !result.OK || len(calls) != 2 || calls[1].method != "PATCH" {
			t.Fatalf("result %+v, calls %+v, output:\n%s", result, calls, out)
		}
		if body, _ := calls[1].body.(map[string]any); body["content"] != "new text" {
			t.Errorf("sent %+v", calls[1].body)
		}
		if result, out := run(`chat edit 01CONV 9 --content "x"`); result.OK || !strings.Contains(out, "no message 9") {
			t.Fatalf("a missing message number: result %+v, output:\n%s", result, out)
		}
	})
}
