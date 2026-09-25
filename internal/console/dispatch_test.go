package console

import (
	"context"
	"net/http"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// Over SSH the dispatcher runs on the console's own goroutine, outside every
// piece of the server's middleware, so a handler that panics must come back
// as a failed command — not end the process for everybody.
func TestADispatchedPanicIsAFailedCommand(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) {
		var nothing []string
		_ = nothing[3]
	})
	response, err := NewDispatcher(mux)(context.Background(), user.User{ID: "u1"}, http.MethodGet, "/boom", nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Status)
	}
}
