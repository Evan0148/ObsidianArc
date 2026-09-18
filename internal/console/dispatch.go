package console

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/auth"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// Dispatcher performs one admin API call as actor. NewDispatcher is the
// only production implementation; a test may supply its own to observe or
// refuse specific calls without standing up a real mux.
type Dispatcher func(ctx context.Context, actor user.User, method, path string, body any) (Response, error)

// recorder is an in-process http.ResponseWriter with no socket behind it —
// header map, status, buffer. httpx has no exported recorder (its own is
// the access log's private type), and the contract is explicit that
// dispatch.go must not reach for net/http/httptest from non-test code, so
// this is the one place that type has to exist.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newRecorder() *recorder {
	return &recorder{header: make(http.Header), status: http.StatusOK}
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) Write(p []byte) (int, error) { return r.body.Write(p) }

func (r *recorder) WriteHeader(status int) { r.status = status }

// Flush satisfies http.Flusher. There is nowhere to flush to — but its mere
// presence is what lets httpx.NewSSE's flushability probe succeed, which is
// what keeps POST /api/admin/health/probe (the one admin route that answers
// text/event-stream instead of JSON) reachable through this dispatcher at
// all. Without it, NewSSE would refuse the recorder and that route would be
// unreachable from the console.
func (r *recorder) Flush() {}

// NewDispatcher builds a Dispatcher that serves one admin API call in
// process, through mux, with no socket, no compression, no access-log entry
// and no same-origin check — exactly the admin handlers themselves, wired
// the same way admin.Handlers.Routes is everywhere else, so routing through
// a real *http.ServeMux is what gives them r.PathValue(...). Never bypass
// mux and call a handler directly: the whole permission guarantee is that
// auth.RequireAdmin and the route's own hasPermission wrapper run exactly
// as they would for a browser request.
func NewDispatcher(mux http.Handler) Dispatcher {
	return func(ctx context.Context, actor user.User, method, path string, body any) (Response, error) {
		var reader io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				return Response{}, fmt.Errorf("console: encode request body: %w", err)
			}
			reader = bytes.NewReader(encoded)
		}

		req, err := http.NewRequestWithContext(auth.WithUser(ctx, actor), method, path, reader)
		if err != nil {
			return Response{}, fmt.Errorf("console: build request: %w", err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		// There is no real peer to read one from; a handler that wants the
		// caller's address reads it from Session.IP through the command,
		// not from the request.
		req.RemoteAddr = "127.0.0.1:0"

		rec := newRecorder()
		mux.ServeHTTP(rec, req)
		return Response{Status: rec.status, Body: rec.body.Bytes()}, nil
	}
}

// SSEFrame is one frame of a text/event-stream response. ParseSSE exists
// for the one admin route that is not JSON, POST /api/admin/health/probe:
// the recorder above has no socket to stream over, so it simply
// accumulates every frame httpx.SSE writes, in order, into Response.Body —
// this turns that accumulation back into the frames a real client would
// have seen one at a time, so a `health probe` command (added once
// cmd_ops.go exists) does not need to know the wire format itself.
type SSEFrame struct {
	Event string
	Data  []byte
}

// ParseSSE decodes a Response.Body captured from an SSE route.
func ParseSSE(body []byte) []SSEFrame {
	var frames []SSEFrame
	for _, block := range strings.Split(string(body), "\n\n") {
		block = strings.TrimRight(block, "\n")
		if block == "" {
			continue
		}
		var frame SSEFrame
		var data []string
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				frame.Event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = append(data, strings.TrimPrefix(line, "data: "))
			}
		}
		if frame.Event == "" && len(data) == 0 {
			continue // a `: comment` keepalive, not a real frame
		}
		frame.Data = []byte(strings.Join(data, "\n"))
		frames = append(frames, frame)
	}
	return frames
}
