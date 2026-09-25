package invite

import (
	"context"
	"net/http"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/auth"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/httpx"
)

// Handlers is an account's own view of invite codes: its personal code, who
// has used it, and the reward it has earned. Nothing here is administrative
// — every route sits behind auth.RequireUser alone, the same as notify and
// card's account-facing routes — and the console dispatches into these same
// handlers, so they need no browser session either.
type Handlers struct {
	store *Store
}

func NewHandlers(store *Store) *Handlers { return &Handlers{store: store} }

func (h *Handlers) Routes(mux *http.ServeMux) {
	mux.Handle("GET /api/profile/invites", auth.RequireUser(httpx.Wrap(h.profile)))
	mux.Handle("POST /api/profile/invites/regenerate", auth.RequireUser(httpx.Wrap(h.regenerate)))
}

// payload builds the one shape both routes answer with: GET reads it as it
// stands, and regenerate answers with it fresh once the new code exists, so
// a client never has to follow one response with another just to redraw the
// panel.
func (h *Handlers) payload(ctx context.Context, accountID string) (map[string]any, error) {
	enabled, limit, rewardCards, rewardCardDays := h.store.Settings()
	out := map[string]any{
		"enabled": enabled, "code": "", "limit": limit, "used": 0,
		"reward_cards": rewardCards, "reward_card_days": rewardCardDays,
		"invitees": []any{},
	}
	if !enabled {
		return out, nil
	}

	code, err := h.store.PersonalCode(ctx, accountID)
	if err != nil {
		return nil, err
	}
	uses, err := h.store.Uses(ctx, code.ID)
	if err != nil {
		return nil, err
	}

	out["code"] = code.Code
	out["used"] = code.Uses
	invitees := make([]map[string]any, 0, len(uses))
	for _, use := range uses {
		invitees = append(invitees, map[string]any{
			"nickname": use.Nickname, "username": use.Username, "created_at": use.CreatedAt,
			"rewarded":       use.RewardedAt != 0 && use.RewardSkipped == "",
			"reward_skipped": use.RewardSkipped,
		})
	}
	out["invitees"] = invitees
	return out, nil
}

func (h *Handlers) profile(w http.ResponseWriter, r *http.Request) error {
	account := auth.MustUser(r.Context())
	out, err := h.payload(r.Context(), account.ID)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) regenerate(w http.ResponseWriter, r *http.Request) error {
	account := auth.MustUser(r.Context())
	if enabled, _, _, _ := h.store.Settings(); !enabled {
		return httpx.ForbiddenCode("invites_disabled", "Personal invite codes are switched off on this server.")
	}
	if _, err := h.store.Regenerate(r.Context(), account.ID); err != nil {
		return httpx.Internal(err)
	}
	out, err := h.payload(r.Context(), account.ID)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.WriteJSON(w, http.StatusOK, out)
}
