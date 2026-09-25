package console

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/id"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// In the browser the backoffice draws a place to type the code; here the code
// is typed by hand, so the refusal has to name the command that takes it —
// in the reader's language, not the server's English sentence.
func TestAnAdministrativeCommandSaysHowToUnlockIt(t *testing.T) {
	actor := user.User{ID: id.New(), Username: "root", Role: user.RoleSuperAdmin}
	refusal := `{"error":{"code":"two_factor_backoffice_verify","message":"Enter a code from your authenticator app to open the backoffice."}}`
	c := New(Options{Dispatch: func(_ context.Context, _ user.User, _, _ string, _ any) (Response, error) {
		return Response{Status: 403, Body: []byte(refusal)}, nil
	}})

	for lang, want := range map[string]string{"en": "2fa backoffice <code>", "zh": "2fa backoffice <验证码>"} {
		var out bytes.Buffer
		result := c.Execute(context.Background(), &Session{Actor: actor, Transport: "ssh", Lang: lang, Width: 100}, &out, "dash")
		if result.OK {
			t.Fatalf("%s: a refused command reported success", lang)
		}
		if !strings.Contains(out.String(), want) {
			t.Errorf("%s: the refusal does not say how to unlock:\n%s", lang, out.String())
		}
	}
}
