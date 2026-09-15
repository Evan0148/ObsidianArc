package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/health"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/id"
)

func TestUptimeSeparatesHourAndDay(t *testing.T) {
	for _, tc := range []struct {
		name      string
		recent    bool
		reset     bool
		wantTotal int
		wantDay   float64
	}{
		{name: "recent failure against earlier successes", recent: true, wantTotal: 3, wantDay: 2.0 / 3},
		{name: "quiet hour still has day history", wantTotal: 2, wantDay: 1},
		{name: "reset excludes old evidence from both windows", recent: true, reset: true, wantTotal: 1},
		{name: "reset without new evidence", reset: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := newInstance(t)
			admin := in.register("uptime_admin", "password123")
			provider := decode[struct {
				Provider struct{ ID string }
			}](t, in.do(http.MethodPost, "/api/admin/providers", map[string]any{
				"name": "Uptime provider", "kind": "openai", "base_url": "https://example.com/v1", "api_key": "test",
			}, admin))
			model := decode[struct {
				Model struct{ ID string }
			}](t, in.do(http.MethodPost, "/api/admin/models", map[string]any{
				"provider_id": provider.Provider.ID, "model_id": "uptime-model", "display_name": "Uptime model", "enabled": true,
			}, admin))
			// Even a probe window longer than a day must not extend the chart
			// or either percentage beyond the period named in the interface.
			if res := in.do(http.MethodPut, "/api/admin/settings", map[string]string{
				"health.window_minutes": "2880",
			}, admin); res.Code != http.StatusOK {
				t.Fatalf("set probe window: %d %s", res.Code, res.Body.String())
			}
			if tc.reset {
				if res := in.do(http.MethodPost, "/api/admin/health/reset", nil, admin); res.Code != http.StatusOK {
					t.Fatalf("reset: %d", res.Code)
				}
			}
			now := time.Now()
			add := func(age time.Duration, ok bool) {
				t.Helper()
				_, err := in.db.Exec(context.Background(),
					`INSERT INTO model_probes (id, model_id, at, ok, code, message, latency_ms) VALUES (?, ?, ?, ?, ?, ?, ?)`,
					id.New(), model.Model.ID, now.Add(-age).UnixMilli(), ok, "", "", 1)
				if err != nil {
					t.Fatal(err)
				}
			}
			add(25*time.Hour, false)
			add(2*time.Hour, true)
			add(90*time.Minute, true)
			if tc.reset {
				add(30*time.Minute, true)
			}
			if tc.recent {
				add(0, false)
			}
			res := in.do(http.MethodGet, "/api/uptime", nil, admin)
			if res.Code != http.StatusOK {
				t.Fatalf("uptime: %d %s", res.Code, res.Body.String())
			}
			body := decode[struct {
				Models []struct {
					Uptime     *float64           `json:"uptime"`
					UptimeHour *float64           `json:"uptime_hour"`
					Total      int                `json:"total"`
					History    []health.TimePoint `json:"history"`
				}
			}](t, res)
			if len(body.Models) != 1 {
				t.Fatalf("models = %+v", body.Models)
			}
			got := body.Models[0]
			if got.Total != tc.wantTotal {
				t.Errorf("daily total = %d, want %d", got.Total, tc.wantTotal)
			}
			if tc.wantTotal == 0 {
				if got.Uptime != nil {
					t.Error("empty day must not report a percentage")
				}
			} else if got.Uptime == nil || *got.Uptime != tc.wantDay {
				t.Errorf("daily rate = %v, want %v", got.Uptime, tc.wantDay)
			}
			if tc.recent {
				if got.UptimeHour == nil || *got.UptimeHour != 0 {
					t.Errorf("failed hour must report zero: %+v", got)
				}
			} else if got.UptimeHour != nil {
				t.Error("empty hour must not borrow the daily percentage")
			}
			if len(got.History) != 24 {
				t.Fatalf("history points = %d", len(got.History))
			}
			if age := now.Sub(time.UnixMilli(got.History[0].At)); age < 23*time.Hour || age > 24*time.Hour {
				t.Errorf("first chart point is %v old, want 23–24h even after reset", age)
			}
			total := 0
			for _, point := range got.History {
				total += point.Total
			}
			if total != tc.wantTotal {
				t.Errorf("chart sample total = %d, want %d", total, tc.wantTotal)
			}
		})
	}
}
