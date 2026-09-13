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
