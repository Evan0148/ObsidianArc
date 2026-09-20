package oauth

import (
	"crypto/hmac"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/auth"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/httpx"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// The two halves of a sign-in are ordinary navigations, not API calls: the
// browser leaves for the provider and comes back, and both ends of that trip
// have to be something a person can land on. So these two answer with a
// redirect even when they fail — a page that says what went wrong in the
// reader's own language, rather than a JSON error rendered raw in a tab.
//
// The other two are API calls like any other, and answer like them.

// statePath scopes the state cookie. Every route below sits under it.
const statePath = "/api/auth/oauth"

type Handlers struct {
	service *Service
	stamp   *stamp

	// Whether the state cookie is marked Secure. Follows the session
	// cookie's own setting, so a plain-HTTP development instance still works
	// and a real one never sends it in the clear.
	SecureCookie bool
	// The address this instance answers at, resolved by the wiring through
	// httpx.PublicOrigin so that every URL this server hands to somebody else
	// agrees. Nil falls back to the request's own host and TLS state, which
	// is wrong behind a proxy and right in a test.
	Origin func(*http.Request) string
	// Who is calling, for the per-address registration limit.
	ClientIP func(*http.Request) string
	// One client for every provider call, so a token exchange reuses the
	// connection the identity lookup just opened.
	Client *http.Client
}

func NewHandlers(service *Service, secret []byte) *Handlers {
	return &Handlers{service: service, stamp: newStamp(secret)}
}

func (h *Handlers) Routes(mux *http.ServeMux) {
	// Public, and necessarily so: nobody signing in has a session yet, and
	// the provider sends the browser back with nothing but the code.
	mux.HandleFunc("GET /api/auth/oauth/start/{provider}", h.start)
	mux.HandleFunc("GET /api/auth/oauth/callback/{provider}", h.callback)

	protected := func(handler httpx.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			auth.RequireUser(httpx.Wrap(handler)).ServeHTTP(w, r)
		}
	}
	mux.HandleFunc("GET /api/auth/oauth/connections", protected(h.connections))
	mux.HandleFunc("DELETE /api/auth/oauth/connections/{provider}", protected(h.disconnect))
}

// --- the browser's round trip -------------------------------------------------

func (h *Handlers) start(w http.ResponseWriter, r *http.Request) {
	provider := ByID(r.PathValue("provider"))
	if provider == nil {
		httpx.WriteError(w, r, httpx.NotFound("No such sign-in provider."))
		return
	}

	// Where a failure lands, decided before anything can fail: a person
	// adding a connection belongs back in their settings, and a person
	// signing in belongs at the sign-in card.
	account, signedIn := auth.UserFrom(r.Context())
	linking := signedIn && r.URL.Query().Get("link") == "1"

	if !h.service.Enabled(provider.ID) {
		h.fail(w, r, linking, "unavailable")
		return
	}

	value := state{
		Provider: provider.ID,
		Nonce:    token(),
		Next:     safeNext(r.URL.Query().Get("next")),
		Expiry:   time.Now().Add(stateTTL).UnixMilli(),
	}
	if linking {
		value.UserID = account.ID
	}
	challenge := ""
	if provider.PKCE {
		value.Verifier = token()
		challenge = challengeFor(value.Verifier)
	}

	cookie, err := h.stamp.issue(value)
	if err != nil {
		h.fail(w, r, linking, "failed")
		return
	}
	h.setState(w, cookie)

	target := provider.authorise(h.service.Credentials(provider.ID),
		h.redirectURI(r, provider.ID), value.Nonce, challenge)
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handlers) callback(w http.ResponseWriter, r *http.Request) {
	provider := ByID(r.PathValue("provider"))
	if provider == nil {
		httpx.WriteError(w, r, httpx.NotFound("No such sign-in provider."))
		return
	}

	// Spent either way. A state that has come back is finished whether the
	// sign-in worked or not, and leaving it behind would leave a replayable
	// one in the browser.
	cookie, err := r.Cookie(stateCookie)
	h.clearState(w)
	if err != nil {
		h.fail(w, r, false, "state")
		return
	}
	value, err := h.stamp.read(cookie.Value)
	if err != nil {
		h.fail(w, r, false, "state")
		return
	}
	linking := value.UserID != ""

	query := r.URL.Query()
	// The provider refusing, or the person pressing cancel on the consent
	// screen. Its own words are not shown: they are English, sometimes
	// untranslatable, and "you did not authorise this" is the whole of it.
	if query.Get("error") != "" {
		h.fail(w, r, linking, "denied")
		return
	}
	if value.Provider != provider.ID ||
		!hmac.Equal([]byte(query.Get("state")), []byte(value.Nonce)) {
		h.fail(w, r, linking, "state")
		return
	}
	code := query.Get("code")
	if code == "" {
		h.fail(w, r, linking, "denied")
		return
	}
	if !h.service.Enabled(provider.ID) {
		// Switched off while somebody was at the consent screen.
		h.fail(w, r, linking, "unavailable")
		return
	}

	identity, err := provider.Authenticate(r.Context(), h.Client,
		h.service.Credentials(provider.ID), code, h.redirectURI(r, provider.ID), value.Verifier)
	if err != nil {
		h.fail(w, r, linking, providerFailure(err))
		return
	}

	if linking {
		h.finishLink(w, r, value, identity)
		return
	}

	account, err := h.service.SignIn(r.Context(), identity, h.address(r), r.UserAgent())
	if err != nil {
		h.fail(w, r, false, signInFailure(err))
		return
	}
	token, err := h.service.auth.StartSession(r.Context(), account, h.address(r), r.UserAgent())
	if err != nil {
		h.fail(w, r, false, "failed")
		return
	}
	h.service.auth.SetCookie(w, token)

	next := value.Next
	if next == "" {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusFound)
}

// finishLink attaches a provider to the account that started the flow.
//
// The account is the one named in the signed state, and it has to still be
// the one holding the session: a browser that signed out and in as somebody
// else between the two halves must not hand that somebody else the
// connection.
func (h *Handlers) finishLink(w http.ResponseWriter, r *http.Request, value state, identity Identity) {
	account, signedIn := auth.UserFrom(r.Context())
	if !signedIn || account.ID != value.UserID {
		h.fail(w, r, true, "state")
		return
	}
	if err := h.service.Connect(r.Context(), account.ID, identity); err != nil {
		if errors.Is(err, ErrAlreadyLinked) {
			h.fail(w, r, true, "already_linked")
			return
		}
		h.fail(w, r, true, "failed")
		return
	}
	http.Redirect(w, r, "/settings?oauth=connected", http.StatusFound)
}

// fail sends the browser to a page that can say what happened.
//
// A code rather than a sentence, for the reason every other refusal on this
// path carries one: the server has no idea what language the reader has the
// interface in.
func (h *Handlers) fail(w http.ResponseWriter, r *http.Request, linking bool, code string) {
	page := "/login"
	if linking {
		page = "/settings"
	}
	http.Redirect(w, r, page+"?oauth_error="+url.QueryEscape(code), http.StatusFound)
}

func providerFailure(err error) string {
	switch {
	case errors.Is(err, ErrDenied):
		return "denied"
	case errors.Is(err, ErrNotConfigured):
		return "unavailable"
	default:
		return "provider"
	}
}

func signInFailure(err error) string {
	var throttled *auth.SignupThrottleError
	if errors.As(err, &throttled) {
		return "throttled"
	}
	var domain *auth.EmailDomainError
	if errors.As(err, &domain) {
		return "domain"
	}
	switch {
	case errors.Is(err, ErrAddressTaken):
		return "address_taken"
	case errors.Is(err, ErrSignupClosed):
		return "signup_closed"
	case errors.Is(err, auth.ErrRegistrationClosed):
		return "registration_closed"
	case errors.Is(err, auth.ErrAccountDisabled):
		return "disabled"
	case errors.Is(err, auth.ErrSignupIPBlocked):
		return "ip_blocked"
	case errors.Is(err, auth.ErrEmailRequired):
		return "email_required"
	case errors.Is(err, user.ErrQQRequired):
		return "qq_required"
	case errors.Is(err, user.ErrEmailTaken):
		return "address_taken"
	default:
		return "failed"
	}
}

// --- the account's own screen -------------------------------------------------

func (h *Handlers) connections(w http.ResponseWriter, r *http.Request) error {
	account := auth.MustUser(r.Context())
	linked, hasPassword, err := h.service.Connections(r.Context(), account.ID)
	if err != nil {
		return httpx.Internal(err)
	}

	offered := make([]map[string]any, 0, len(Providers()))
	for _, provider := range Providers() {
		offered = append(offered, map[string]any{
			"id":      provider.ID,
			"name":    provider.Name,
			"enabled": h.service.Enabled(provider.ID),
		})
	}
	return httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"connections": linked,
		"providers":   offered,
		// What decides whether a connection may be removed, and whether the
		// password box says "set" or "change".
		"has_password": hasPassword,
	})
}

func (h *Handlers) disconnect(w http.ResponseWriter, r *http.Request) error {
	account := auth.MustUser(r.Context())
	provider := ByID(r.PathValue("provider"))
	if provider == nil {
		return httpx.NotFound("No such sign-in provider.")
	}

	switch err := h.service.Disconnect(r.Context(), account.ID, provider.ID); {
	case err == nil:
		return httpx.NoContent(w)
	case errors.Is(err, ErrLastWayIn):
		return httpx.Conflict("last_way_in",
			"Set a password first — this is the only way left into this account.")
	case errors.Is(err, ErrNotConnected):
		return httpx.NotFound("That provider is not connected to this account.")
	default:
		return httpx.Internal(err)
	}
}

// --- where the provider sends the browser back --------------------------------

// redirectURI is the address this instance answers the provider at.
//
// Built from the request unless an operator has configured a public URL,
// which sounds like trusting a header and is not: the provider will only
// redirect to a URI registered in its own console, and the same value is sent
// again at the exchange, where it has to match the one the code was issued
// for. A caller who tampers with either only breaks their own sign-in — there
// is no host they can name here that would receive anything.
func (h *Handlers) redirectURI(r *http.Request, providerID string) string {
	if h.Origin != nil {
		return h.Origin(r) + statePath + "/callback/" + providerID
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + statePath + "/callback/" + providerID
}

func (h *Handlers) address(r *http.Request) string {
	if h.ClientIP == nil {
		return ""
	}
	return h.ClientIP(r)
}
