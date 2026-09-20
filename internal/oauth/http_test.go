package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/auth"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// stub points a provider at a server this test controls, and restores it
// afterwards. The endpoints are constants in the source for a reason — an
// operator must not be able to move them — so the only place they can be
// moved is here, inside the package.
func stub(t *testing.T, providerID string, tokenHandler http.HandlerFunc, identity Identity) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(tokenHandler)
	t.Cleanup(server.Close)

	provider := ByID(providerID)
	auth, token, identify := provider.AuthURL, provider.TokenURL, provider.identify
	t.Cleanup(func() {
		provider.AuthURL, provider.TokenURL, provider.identify = auth, token, identify
	})
	provider.AuthURL = server.URL + "/authorize"
	provider.TokenURL = server.URL + "/token"
	provider.identify = func(context.Context, *http.Client, string) (Identity, error) {
		return identity, nil
	}
	return server
}

func handlers(t *testing.T, f *fixture) (*Handlers, *http.ServeMux) {
	t.Helper()
	h := NewHandlers(f.service, []byte("an-instance-secret"))
	h.Client = http.DefaultClient
	mux := http.NewServeMux()
	h.Routes(mux)
	return h, mux
}

func get(mux *http.ServeMux, path string, cookies []*http.Cookie, account *user.User) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	if account != nil {
		request = request.WithContext(auth.WithUser(request.Context(), *account))
	}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	return recorder
}

// The whole round trip: the browser leaves with a signed state and comes back
// with a code, and what it gets is a session of this instance's own.
func TestTheRoundTripSignsSomebodyIn(t *testing.T) {
	f := newFixture(t)
	f.configure(t, "github")

	var exchanged url.Values
	stub(t, "github", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		exchanged = r.PostForm
		_, _ = w.Write([]byte(`{"access_token":"a-token","token_type":"bearer"}`))
	}, Identity{Subject: "4218", Login: "octocat", Name: "The Octocat", Email: "cat@example.com"})

	_, mux := handlers(t, f)

	start := get(mux, "/api/auth/oauth/start/github", nil, nil)
	if start.Code != http.StatusFound {
		t.Fatalf("start = %d %s", start.Code, start.Body.String())
	}
	target, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	nonce := target.Query().Get("state")
	if nonce == "" {
		t.Fatal("the browser was sent to the provider with no state")
	}

	cookies := start.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %+v, want just the state", cookies)
	}
	state := cookies[0]
	if !state.HttpOnly {
		t.Error("the state cookie is readable by page script")
	}
	if state.Path != statePath {
		t.Errorf("state cookie path = %q, want it scoped to %q", state.Path, statePath)
	}
	if strings.Contains(state.Value, nonce) && !strings.Contains(state.Value, ".") {
		t.Error("the state cookie is not signed")
	}

	back := get(mux, "/api/auth/oauth/callback/github?code=the-code&state="+nonce,
		[]*http.Cookie{state}, nil)
	if back.Code != http.StatusFound || back.Header().Get("Location") != "/" {
		t.Fatalf("callback = %d %s", back.Code, back.Header().Get("Location"))
	}

	// The exchange sent what the provider checks the code against.
	if exchanged.Get("code") != "the-code" ||
		exchanged.Get("client_id") != "a-client-id" ||
		exchanged.Get("client_secret") != "a-client-secret" {
		t.Errorf("exchange = %v, want the code and this instance's credentials", exchanged)
	}
	if !strings.HasSuffix(exchanged.Get("redirect_uri"), "/api/auth/oauth/callback/github") {
		t.Errorf("redirect_uri = %q, want the callback it came back to", exchanged.Get("redirect_uri"))
	}

	// A session, and the state spent.
	var session, cleared *http.Cookie
	for _, cookie := range back.Result().Cookies() {
		switch cookie.Name {
		case "obsidian_session":
			session = cookie
		case stateCookie:
			cleared = cookie
		}
	}
	if session == nil || session.Value == "" {
		t.Fatal("the callback issued no session")
	}
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Error("the state cookie was left behind for a second use")
	}

	account, _, err := f.auth.Authenticate(context.Background(), session.Value)
	if err != nil {
		t.Fatalf("the session does not resolve: %v", err)
	}
	if account.Username != "octocat" {
		t.Errorf("signed in as %q, want the account the provider named", account.Username)
	}
}

// Everything that can be wrong with the value that comes back, and the same
// answer to all of it. Each case has to leave no account behind: a callback
// that cannot be verified must not be a way to open one.
func TestACallbackThatCannotBeVerifiedSignsNobodyIn(t *testing.T) {
	f := newFixture(t)
	f.configure(t, "github")
	stub(t, "github", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"a-token"}`))
	}, Identity{Subject: "4218", Login: "octocat"})
	_, mux := handlers(t, f)

	start := get(mux, "/api/auth/oauth/start/github", nil, nil)
	state := start.Result().Cookies()[0]
	target, _ := url.Parse(start.Header().Get("Location"))
	nonce := target.Query().Get("state")

	other := newFixture(t)
	other.configure(t, "github")
	elsewhere := get(func() *http.ServeMux { _, m := handlers(t, other); return m }(),
		"/api/auth/oauth/start/github", nil, nil).Result().Cookies()[0]

	cases := map[string]struct {
		query   string
		cookies []*http.Cookie
	}{
		"no cookie at all":      {"?code=c&state=" + nonce, nil},
		"a state nobody sent":   {"?code=c&state=invented", []*http.Cookie{state}},
		"no state at all":       {"?code=c", []*http.Cookie{state}},
		"no code":               {"?state=" + nonce, []*http.Cookie{state}},
		"the provider refused":  {"?error=access_denied&state=" + nonce, []*http.Cookie{state}},
		"a cookie from nowhere": {"?code=c&state=" + nonce, []*http.Cookie{{Name: stateCookie, Value: "forged.value"}}},
		"another instance's":    {"?code=c&state=" + nonce, []*http.Cookie{elsewhere}},
		"a state for a sibling": {"?code=c&state=" + nonce, []*http.Cookie{state}},
	}
	for name, testCase := range cases {
		path := "/api/auth/oauth/callback/github" + testCase.query
		if name == "a state for a sibling" {
			// The same signed state, presented at the other provider's
			// callback: the cookie says github and this is not github.
			path = "/api/auth/oauth/callback/google" + testCase.query
		}
		response := get(mux, path, testCase.cookies, nil)
		if response.Code != http.StatusFound {
			t.Errorf("%s: %d", name, response.Code)
			continue
		}
		location := response.Header().Get("Location")
		if !strings.HasPrefix(location, "/login?oauth_error=") {
			t.Errorf("%s: sent to %q, want the sign-in page with a reason", name, location)
		}
		for _, cookie := range response.Result().Cookies() {
			if cookie.Name == "obsidian_session" && cookie.Value != "" {
				t.Errorf("%s: a session was issued anyway", name)
			}
		}
	}

	total, err := f.users.Count(context.Background(), nil)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 0 {
		t.Errorf("accounts = %d, want none opened by a callback nobody could verify", total)
	}
}

func TestAProviderThatIsNotOnLeadsNowhere(t *testing.T) {
	f := newFixture(t)
	_, mux := handlers(t, f)

	response := get(mux, "/api/auth/oauth/start/github", nil, nil)
	if response.Code != http.StatusFound ||
		response.Header().Get("Location") != "/login?oauth_error=unavailable" {
		t.Errorf("start = %d %s, want the sign-in page saying so",
			response.Code, response.Header().Get("Location"))
	}
	if len(response.Result().Cookies()) != 0 {
		t.Error("a state was written for a sign-in that cannot start")
	}
	if missing := get(mux, "/api/auth/oauth/start/nonesuch", nil, nil); missing.Code != http.StatusNotFound {
		t.Errorf("an invented provider = %d, want 404", missing.Code)
	}
}

// Adding a connection is a different flow from signing in, and the account it
// lands on is the one that started it — not whoever happens to hold the
// session when the browser comes back.
func TestLinkingBindsToTheAccountThatStartedIt(t *testing.T) {
	f := newFixture(t)
	f.configure(t, "github")
	stub(t, "github", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"a-token"}`))
	}, Identity{Subject: "4218", Login: "octocat"})
	_, mux := handlers(t, f)

	owner, _, err := f.auth.Register(context.Background(), auth.RegisterInput{
		Username: "founder", Password: "a-good-password",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	stranger, _, err := f.auth.Register(context.Background(), auth.RegisterInput{
		Username: "member", Password: "another-password",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	start := get(mux, "/api/auth/oauth/start/github?link=1", nil, &owner)
	state := start.Result().Cookies()[0]
	target, _ := url.Parse(start.Header().Get("Location"))
	nonce := target.Query().Get("state")

	// The same signed state, presented by somebody else's session.
	hijack := get(mux, "/api/auth/oauth/callback/github?code=c&state="+nonce,
		[]*http.Cookie{state}, &stranger)
	if location := hijack.Header().Get("Location"); !strings.HasPrefix(location, "/settings?oauth_error=") {
		t.Fatalf("another account completed the link: %q", location)
	}
	if connections, _, _ := f.service.Connections(context.Background(), stranger.ID); len(connections) != 0 {
		t.Fatalf("the connection landed on the wrong account: %+v", connections)
	}

	back := get(mux, "/api/auth/oauth/callback/github?code=c&state="+nonce,
		[]*http.Cookie{state}, &owner)
	if back.Header().Get("Location") != "/settings?oauth=connected" {
		t.Fatalf("link = %q", back.Header().Get("Location"))
	}
	connections, _, err := f.service.Connections(context.Background(), owner.ID)
	if err != nil {
		t.Fatalf("connections: %v", err)
	}
	if len(connections) != 1 || connections[0].Provider != "github" {
		t.Errorf("connections = %+v, want the one that was just added", connections)
	}
	// Linking does not replace the session it was started from.
	for _, cookie := range back.Result().Cookies() {
		if cookie.Name == "obsidian_session" {
			t.Error("linking a provider re-issued the session")
		}
	}
}

func TestConnectionsAndDisconnectAnswerTheAccountsOwnScreen(t *testing.T) {
	f := newFixture(t)
	f.configure(t, "github")
	_, mux := handlers(t, f)

	account, err := f.service.SignIn(context.Background(), identity("4218", "octocat", ""), "", "")
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}

	response := get(mux, "/api/auth/oauth/connections", nil, &account)
	if response.Code != http.StatusOK {
		t.Fatalf("connections = %d %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{`"provider":"github"`, `"has_password":false`, `"id":"google"`} {
		if !strings.Contains(body, want) {
			t.Errorf("connections = %s, want %s in it", body, want)
		}
	}
	// The subject is how the provider knows the person and says nothing to
	// the person, so it is not in the answer.
	if strings.Contains(body, "4218") {
		t.Errorf("connections = %s, want no provider subject in it", body)
	}

	request := httptest.NewRequest(http.MethodDelete, "/api/auth/oauth/connections/github", nil)
	request = request.WithContext(auth.WithUser(request.Context(), account))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "last_way_in") {
		t.Errorf("disconnect = %d %s, want it refused as the last way in",
			recorder.Code, recorder.Body.String())
	}
}

func TestTheAccountsOwnScreenNeedsASession(t *testing.T) {
	f := newFixture(t)
	_, mux := handlers(t, f)

	if response := get(mux, "/api/auth/oauth/connections", nil, nil); response.Code != http.StatusUnauthorized {
		t.Errorf("connections without a session = %d, want 401", response.Code)
	}
	request := httptest.NewRequest(http.MethodDelete, "/api/auth/oauth/connections/github", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("disconnect without a session = %d, want 401", recorder.Code)
	}
}
