package feedback

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/auth"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/turnstile"
)

func post(t *testing.T, mux *http.ServeMux, actorCtx context.Context, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/feedback", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(actorCtx)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	return recorder
}

// The widget in the panel is presentation; this is the boundary that makes it
// a requirement. A caller that skips the dialog must not get a row, and the
// token that passes is bound to the caller's own address.
func TestSendingCanRequireTurnstile(t *testing.T) {
	store, author, _ := fixture(t)
	handlers := NewHandlers(store)
	ctx := auth.WithUser(context.Background(), author)

	const address = "203.0.113.12"
	verified := 0
	handlers.ClientIP = func(*http.Request) string { return address }
	handlers.Challenge = turnstile.Gate{
		Enabled: func() bool { return true },
		Verify: func(_ context.Context, token, ip string) error {
			verified++
			if ip != address {
				t.Errorf("challenge IP = %q, want %q", ip, address)
			}
			if token != "solved" {
				return turnstile.ErrFailed
			}
			return nil
		},
	}

	mux := http.NewServeMux()
	handlers.Routes(mux)

	refused := post(t, mux, ctx, `{"title":"No proof","body":"…","kind":"bug","priority":"low"}`)
	if refused.Code != http.StatusForbidden ||
		!strings.Contains(refused.Body.String(), `"code":"challenge_failed"`) {
		t.Fatalf("sending without proof = %d %s", refused.Code, refused.Body.String())
	}
	// Refused before the write, so a failed challenge costs the sender none
	// of their daily allowance.
	if _, total, err := store.List(context.Background(), nil, Filter{}); err != nil || total != 0 {
		t.Fatalf("rows after a refused challenge = %d (err %v), want none", total, err)
	}

	accepted := post(t, mux, ctx, `{"title":"With proof","body":"…","kind":"bug","priority":"low","turnstile":"solved"}`)
	if accepted.Code != http.StatusCreated {
		t.Fatalf("sending with proof = %d %s", accepted.Code, accepted.Body.String())
	}
	if verified != 2 {
		t.Errorf("challenge checks = %d, want 2", verified)
	}
}

// The zero Gate is the default wiring on an instance that has never switched
// the challenge on, and it must let an ordinary report through.
func TestSendingIsUnchallengedByDefault(t *testing.T) {
	store, author, _ := fixture(t)
	handlers := NewHandlers(store)
	mux := http.NewServeMux()
	handlers.Routes(mux)

	response := post(t, mux, auth.WithUser(context.Background(), author),
		`{"title":"Plain","body":"Something happened.","kind":"idea","priority":"high"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("sending = %d %s", response.Code, response.Body.String())
	}
}

// The form is what somebody typed, so a refusal has to be a sentence about
// what to change rather than a 500.
func TestMalformedReportsAreRefusedWithAReason(t *testing.T) {
	store, author, _ := fixture(t)
	handlers := NewHandlers(store)
	mux := http.NewServeMux()
	handlers.Routes(mux)
	ctx := auth.WithUser(context.Background(), author)

	for name, body := range map[string]string{
		"no title":     `{"title":"  ","body":"x"}`,
		"no body":      `{"title":"x","body":"   "}`,
		"unknown kind": `{"title":"x","body":"y","kind":"question"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if response := post(t, mux, ctx, body); response.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (%s)", response.Code, response.Body.String())
			}
		})
	}
}

// What the panel reads to say "three more today" before somebody writes five
// hundred words into a box that cannot be sent.
func TestTheListSaysHowMuchOfTheDayIsLeft(t *testing.T) {
	store, author, _ := fixture(t)
	handlers := NewHandlers(store)
	mux := http.NewServeMux()
	handlers.Routes(mux)
	ctx := auth.WithUser(context.Background(), author)

	for i := 0; i < 2; i++ {
		if response := post(t, mux, ctx, `{"title":"x","body":"y"}`); response.Code != http.StatusCreated {
			t.Fatalf("send %d = %d", i, response.Code)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/api/feedback", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list = %d", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"remaining":`+strconv.Itoa(MaxPerDay-2)) {
		t.Errorf("list body = %s, want %d remaining", body, MaxPerDay-2)
	}
}
