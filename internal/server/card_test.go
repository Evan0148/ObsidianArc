package server

import (
	"net/http"
	"testing"
	"time"
)

// The administrator chooses an absolute expiry in the browser. Exercise the
// HTTP boundary so a renamed JSON field cannot quietly turn it back into the
// old implicit thirty-day grant.
func TestAdministratorGrantsCardsWithChosenExpiry(t *testing.T) {
	in := newInstance(t)
	admin := in.register("card-admin", "a-good-password")
	reader := in.register("card-reader", "a-good-password")
	expires := time.Now().Add(45 * 24 * time.Hour).Truncate(time.Second).UnixMilli()

	response := in.do(http.MethodPost, "/api/admin/users/"+reader.userID+"/cards",
		map[string]any{"cards": 2, "expires_at": expires}, admin)
	if response.Code != http.StatusCreated {
		t.Fatalf("grant cards: %d %s", response.Code, response.Body.String())
	}

	detail := decode[struct {
		Cards struct {
			Available int `json:"available"`
			Cards     []struct {
				ExpiresAt int64 `json:"expires_at"`
			} `json:"cards"`
		} `json:"cards"`
	}](t, in.do(http.MethodGet, "/api/admin/users/"+reader.userID, nil, admin))
	if detail.Cards.Available != 2 || len(detail.Cards.Cards) != 2 {
		t.Fatalf("holding = %+v, want two available cards", detail.Cards)
	}
	for _, record := range detail.Cards.Cards {
		if record.ExpiresAt != expires {
			t.Errorf("expires_at = %d, want %d", record.ExpiresAt, expires)
		}
	}

	past := in.do(http.MethodPost, "/api/admin/users/"+reader.userID+"/cards",
		map[string]any{"cards": 1, "expires_at": time.Now().Add(-time.Minute).UnixMilli()}, admin)
	if past.Code != http.StatusBadRequest {
		t.Fatalf("past expiry: %d %s", past.Code, past.Body.String())
	}
}

// A card that ran out is the one an operator is asked to fix, and the reader
// has to be able to spend it afterwards — which is the whole point and the
// part a store-level test cannot see, because it never crosses the handler
// that decides whose cards these are.
func TestAdministratorMovesALapsedCardBackIntoUse(t *testing.T) {
	in := newInstance(t)
	admin := in.register("move-admin", "a-good-password")
	reader := in.register("move-reader", "a-good-password")

	soon := time.Now().Add(2 * time.Second).UnixMilli()
	granted := in.do(http.MethodPost, "/api/admin/users/"+reader.userID+"/cards",
		map[string]any{"cards": 1, "expires_at": soon}, admin)
	if granted.Code != http.StatusCreated {
		t.Fatalf("grant card: %d %s", granted.Code, granted.Body.String())
	}

	// Retire it the way time would, rather than by waiting for it.
	if _, err := in.db.Exec(t.Context(),
		`UPDATE usage_cards SET expires_at = ? WHERE user_id = ?`,
		time.Now().Add(-time.Hour).UnixMilli(), reader.userID); err != nil {
		t.Fatal(err)
	}
	if mine := readerCards(t, in, reader); len(mine) != 0 {
		t.Fatalf("an expired card was still offered to its owner: %+v", mine)
	}

	later := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second).UnixMilli()
	moved := decode[struct {
		Moved int `json:"moved"`
	}](t, in.do(http.MethodPatch, "/api/admin/users/"+reader.userID+"/cards",
		map[string]any{"expires_at": later}, admin))
	if moved.Moved != 1 {
		t.Fatalf("moved = %d, want 1", moved.Moved)
	}

	mine := readerCards(t, in, reader)
	if len(mine) != 1 || mine[0].ExpiresAt != later {
		t.Fatalf("the owner sees %+v, want one card expiring at %d", mine, later)
	}
	spent := in.do(http.MethodPost, "/api/usage/cards/"+mine[0].ID+"/use", map[string]any{}, reader)
	if spent.Code != http.StatusNoContent {
		t.Fatalf("spend the moved card: %d %s", spent.Code, spent.Body.String())
	}

	past := in.do(http.MethodPatch, "/api/admin/users/"+reader.userID+"/cards",
		map[string]any{"expires_at": time.Now().Add(-time.Minute).UnixMilli()}, admin)
	if past.Code != http.StatusBadRequest {
		t.Fatalf("past expiry: %d %s", past.Code, past.Body.String())
	}
}

// What the account itself is offered, which is the only view that says
// whether a moved card is usable again.
func readerCards(t *testing.T, in *instance, as *session) []struct {
	ID        string `json:"id"`
	ExpiresAt int64  `json:"expires_at"`
} {
	t.Helper()
	return decode[struct {
		Cards []struct {
			ID        string `json:"id"`
			ExpiresAt int64  `json:"expires_at"`
		} `json:"cards"`
	}](t, in.do(http.MethodGet, "/api/usage/cards", nil, as)).Cards
}

// Withdrawing a card, and the part that is a decision rather than a
// mechanism: the account is told nothing.
//
// Cards arrive without a message, so they leave without one. An operator
// correcting a grant they typed wrong is not making an announcement, and the
// assertion is here rather than in a comment because a notice is exactly the
// sort of thing somebody adds later meaning well.
func TestWithdrawingACardIsSilent(t *testing.T) {
	in := newInstance(t)
	admin := in.register("drop-admin", "a-good-password")
	reader := in.register("drop-reader", "a-good-password")

	expires := time.Now().Add(30 * 24 * time.Hour).UnixMilli()
	if res := in.do(http.MethodPost, "/api/admin/users/"+reader.userID+"/cards",
		map[string]any{"cards": 2, "expires_at": expires}, admin); res.Code != http.StatusCreated {
		t.Fatalf("grant cards: %d %s", res.Code, res.Body.String())
	}

	mine := readerCards(t, in, reader)
	if len(mine) != 2 {
		t.Fatalf("the owner sees %d cards, want 2", len(mine))
	}
	doomed := mine[0].ID

	dropped := in.do(http.MethodDelete,
		"/api/admin/users/"+reader.userID+"/cards/"+doomed, nil, admin)
	if dropped.Code != http.StatusNoContent {
		t.Fatalf("withdraw: %d %s", dropped.Code, dropped.Body.String())
	}

	left := readerCards(t, in, reader)
	if len(left) != 1 || left[0].ID == doomed {
		t.Fatalf("the owner still sees %+v", left)
	}
	if spent := in.do(http.MethodPost, "/api/usage/cards/"+doomed+"/use",
		map[string]any{}, reader); spent.Code != http.StatusNotFound {
		t.Fatalf("a withdrawn card was still spendable: %d %s", spent.Code, spent.Body.String())
	}

	// Nothing was posted to the one channel this product has for telling an
	// account something happened.
	feed := decode[struct {
		Announcements []struct {
			ID string `json:"id"`
		} `json:"announcements"`
	}](t, in.do(http.MethodGet, "/api/announcements", nil, reader))
	if len(feed.Announcements) != 0 {
		t.Errorf("withdrawing a card announced something: %+v", feed.Announcements)
	}

	// A spent card is a record, not a permission, and stays.
	remaining := readerCards(t, in, reader)[0].ID
	if used := in.do(http.MethodPost, "/api/usage/cards/"+remaining+"/use",
		map[string]any{}, reader); used.Code != http.StatusNoContent {
		t.Fatalf("spend the other card: %d %s", used.Code, used.Body.String())
	}
	refused := in.do(http.MethodDelete,
		"/api/admin/users/"+reader.userID+"/cards/"+remaining, nil, admin)
	if refused.Code != http.StatusConflict {
		t.Fatalf("withdrawing a spent card: %d %s", refused.Code, refused.Body.String())
	}
}
