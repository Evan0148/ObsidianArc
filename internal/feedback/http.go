package feedback

import (
	"errors"
	"net/http"
	"time"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/auth"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/httpx"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/turnstile"
)

// Handlers is the author's half: send one, and see the ones you sent.
// Reading anybody else's, changing a status and deleting live in
// internal/admin, on the same terms every other operator surface does.
type Handlers struct {
	store *Store
	// The human check, where the operator has switched it on. The daily cap
	// is what stops one account flooding the list; this is what stops a
	// script holding somebody's cookie from spending that allowance ten
	// times a day without a person ever being present.
	Challenge turnstile.Gate
	// Resolves the caller's address for the challenge, set by the wiring the
	// way apikey.Handlers.ClientIP is. A nil ClientIP simply means Turnstile
	// is asked without one.
	ClientIP func(*http.Request) string
}

func NewHandlers(store *Store) *Handlers { return &Handlers{store: store} }

func (h *Handlers) Routes(mux *http.ServeMux) {
	mux.Handle("GET /api/feedback", auth.RequireUser(httpx.Wrap(h.list)))
	mux.Handle("POST /api/feedback", auth.RequireUser(httpx.Wrap(h.create)))
}

// Generous next to MaxBodyChars, so a report that is a little too long is
// refused with a sentence about its length rather than by the decoder.
const maxBody = 64 * 1024

type writeRequest struct {
	Kind     string `json:"kind"`
	Priority string `json:"priority"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	// The Turnstile token, where the operator has switched the challenge on
	// for this scene. Ignored when they have not.
	Turnstile string `json:"turnstile"`
}

type listResponse struct {
	Feedback []Feedback `json:"feedback"`
	// What the form needs to tell somebody they have run out for today,
	// before they have written five hundred words into a box.
	Remaining int `json:"remaining"`
	MaxPerDay int `json:"max_per_day"`
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) error {
	account := auth.MustUser(r.Context())
	records, err := h.store.ListFor(r.Context(), nil, account.ID)
	if err != nil {
		return httpx.Internal(err)
	}

	// Counted from the rows already in hand rather than with a second query:
	// ListFor returns the newest fifty, and the day's ten are inside that.
	remaining := MaxPerDay
	cutoff := time.Now().UnixMilli() - DayWindow.Milliseconds()
	for _, record := range records {
		if record.CreatedAt > cutoff {
			remaining--
		}
	}
	if remaining < 0 {
		remaining = 0
	}

	return httpx.WriteJSON(w, http.StatusOK, listResponse{
		Feedback: records, Remaining: remaining, MaxPerDay: MaxPerDay,
	})
}

func (h *Handlers) create(w http.ResponseWriter, r *http.Request) error {
	account := auth.MustUser(r.Context())

	var body writeRequest
	if err := httpx.DecodeJSON(w, r, &body, maxBody); err != nil {
		return err
	}

	// Before the write, so a refused challenge costs nothing and spends none
	// of the sender's daily allowance.
	address := ""
	if h.ClientIP != nil {
		address = h.ClientIP(r)
	}
	if err := h.Challenge.Check(r.Context(), body.Turnstile, address); err != nil {
		return challengeError(err)
	}

	record, err := h.store.Create(r.Context(), account.ID, Input{
		Kind:     Kind(body.Kind),
		Priority: Priority(body.Priority),
		Title:    body.Title,
		Body:     body.Body,
	})
	if err != nil {
		return TranslateError(err)
	}
	return httpx.WriteJSON(w, http.StatusCreated, record)
}

// challengeError says what the sign-up page and the key panel say, in the
// same codes, so one string in the client covers every screen that can draw
// the widget.
func challengeError(err error) error {
	switch {
	case errors.Is(err, turnstile.ErrFailed):
		return httpx.ForbiddenCode("challenge_failed",
			"The verification could not be completed. Try again.")
	case errors.Is(err, turnstile.ErrUnavailable):
		return httpx.UnavailableCode("challenge_unavailable",
			"Verification is unavailable right now. Try again shortly.")
	default:
		return httpx.Internal(err)
	}
}

// TranslateError maps this package's sentinels onto responses, so the admin
// handlers and these read the same way.
func TranslateError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return httpx.NotFound("No such feedback.")
	case errors.Is(err, ErrInvalidTitle):
		return httpx.BadRequest("A title of 1-120 characters is required.")
	case errors.Is(err, ErrInvalidBody):
		return httpx.BadRequest("Please describe it in 1-8000 characters.")
	case errors.Is(err, ErrInvalidKind):
		return httpx.BadRequest("Feedback is either a bug or an idea.")
	case errors.Is(err, ErrInvalidPrio):
		return httpx.BadRequest("Priority must be low, medium or high.")
	case errors.Is(err, ErrInvalidStatus):
		return httpx.BadRequest("Status must be open or resolved.")
	case errors.Is(err, ErrTooMany):
		return httpx.TooManyRequests("feedback_daily_limit",
			"You have sent today's maximum number of reports. Please continue tomorrow.")
	default:
		return httpx.Internal(err)
	}
}
