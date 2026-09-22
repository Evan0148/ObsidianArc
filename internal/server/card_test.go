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
