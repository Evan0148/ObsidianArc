package usage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/config"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/database"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/database/dbtest"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/group"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// A Thursday, half past midnight in Greenwich. Every turn below is placed
// relative to it, so the weekday and hour each one lands in can be worked out
// by hand — which is the whole point of pinning it rather than using now.
var thursday = time.Date(2026, 9, 24, 0, 30, 0, 0, time.UTC).UnixMilli()

const hourMS = int64(3600_000)

type analyticsFixture struct {
	store  *Store
	people map[string]user.User
	groups map[string]group.Group
}

// openAnalytics builds the same small instance on either engine: two groups,
// three accounts — one with a nickname and one without — and five turns
// across two models, a failure and a refusal that never reached a model.
func openAnalytics(t *testing.T, driver string) analyticsFixture {
	t.Helper()
	ctx := context.Background()

	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "analytics.db"), MaxOpenConns: 4, MaxIdleConns: 2}
	if driver == "postgres" {
		cfg = dbtest.Postgres(t, "the usage analytics do integer arithmetic on timestamps and count distinct values, which is where two dialects disagree")
	}
	db, err := database.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	fixture := analyticsFixture{store: NewStore(db), people: map[string]user.User{}, groups: map[string]group.Group{}}
	groups := group.NewStore(db)
	for _, spec := range []group.CreateInput{{Name: "Default", IsDefault: true}, {Name: "Pro"}} {
		created, err := groups.Create(ctx, nil, spec)
		if err != nil {
			t.Fatal(err)
		}
		fixture.groups[spec.Name] = created
	}
	users := user.NewStore(db)
	for _, spec := range []struct{ username, nickname, group string }{
		{"ada", "Ada Lovelace", "Pro"}, {"bob", "", "Default"}, {"cyd", "Cyd", "Default"},
	} {
		account, err := users.Create(ctx, nil, user.CreateInput{
			Username: spec.username, Nickname: spec.nickname, PasswordHash: "x",
			GroupID: fixture.groups[spec.group].ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		fixture.people[spec.username] = account
	}

	turn := func(who, modelID, modelName, providerName string, at int64, input, output int, credits float64, duration int, status Status) {
		t.Helper()
		account := fixture.people[who]
		if err := fixture.store.Write(ctx, Record{
			UserID: account.ID, GroupID: account.GroupID,
			ModelID: modelID, ModelName: modelName, ProviderID: providerName, ProviderName: providerName,
			InputTokens: input, OutputTokens: output, Credits: credits,
			Status: status, StartedAt: at, FinishedAt: at + int64(duration), DurationMS: duration,
		}); err != nil {
			t.Fatal(err)
		}
	}
	turn("ada", "m-opus", "Opus", "Anthropic", thursday, 100, 50, 2, 1000, StatusOK)
	turn("ada", "m-gpt", "GPT", "OpenRouter", thursday+hourMS, 10, 10, 0.5, 3000, StatusOK)
	turn("bob", "m-opus", "Opus", "Anthropic", thursday+2*hourMS, 40, 0, 1, 500, StatusError)
	turn("bob", "", "", "", thursday+3*hourMS, 0, 0, 0, 10, StatusRejected)
	turn("cyd", "m-opus", "Opus", "Anthropic", thursday+25*hourMS, 5, 5, 0.1, 200, StatusOK)
	return fixture
}

// Every analytics query runs on both engines: these are the ones in this
// package doing arithmetic on epoch milliseconds and counting distinct values,
// and a query that is right on SQLite and wrong on Postgres is the failure
// this project has the fewest other ways to notice.
func TestUsageAnalytics(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			fixture := openAnalytics(t, driver)
			ctx := context.Background()
			store := fixture.store

			t.Run("totals count people and models, not rows", func(t *testing.T) {
				totals, err := store.Totals(ctx, Filter{})
				if err != nil {
					t.Fatal(err)
				}
				if totals.Requests != 5 || totals.Errors != 1 {
					t.Errorf("requests/errors = %d/%d, want 5/1", totals.Requests, totals.Errors)
				}
				if totals.Users != 3 {
					t.Errorf("users = %d, want 3", totals.Users)
				}
				// The refusal carries no model, and an empty id is not one.
				if totals.Models != 2 {
					t.Errorf("models = %d, want 2", totals.Models)
				}
				if totals.DurationMS != 4710 {
					t.Errorf("duration = %d, want 4710", totals.DurationMS)
				}
			})

			t.Run("a model's row says who used it and when", func(t *testing.T) {
				rows, err := store.GroupBy(ctx, "model", MetricUsers, Filter{})
				if err != nil {
					t.Fatal(err)
				}
				opus := rows[0]
				if opus.Key != "m-opus" {
					t.Fatalf("ranked by people, the first model is %q, want m-opus: %+v", opus.Key, rows)
				}
				if opus.Users != 3 || opus.Requests != 3 || opus.Models != 1 {
					t.Errorf("opus users/requests/models = %d/%d/%d, want 3/3/1", opus.Users, opus.Requests, opus.Models)
				}
				if opus.Label != "Opus" || opus.Detail != "Anthropic" {
					t.Errorf("opus is labelled %q / %q", opus.Label, opus.Detail)
				}
				if opus.LastAt != thursday+25*hourMS {
					t.Errorf("opus last used at %d, want %d", opus.LastAt, thursday+25*hourMS)
				}
			})

			t.Run("an account's row leads with the name it chose", func(t *testing.T) {
				rows, err := store.GroupBy(ctx, "user", MetricRequests, Filter{})
				if err != nil {
					t.Fatal(err)
				}
				byKey := map[string]Breakdown{}
				for _, row := range rows {
					byKey[row.Key] = row
				}
				ada := byKey[fixture.people["ada"].ID]
				if ada.Label != "Ada Lovelace" || ada.Detail != "ada" {
					t.Errorf("ada is labelled %q / %q", ada.Label, ada.Detail)
				}
				if ada.Models != 2 {
					t.Errorf("ada used %d models, want 2", ada.Models)
				}
				// No nickname: the handle is the only name there is.
				if bo := byKey[fixture.people["bob"].ID]; bo.Label != "bob" {
					t.Errorf("bo is labelled %q, want the handle", bo.Label)
				}
			})

			t.Run("groups are named from the groups table", func(t *testing.T) {
				rows, err := store.GroupBy(ctx, "group", MetricRequests, Filter{})
				if err != nil {
					t.Fatal(err)
				}
				counts := map[string]int64{}
				for _, row := range rows {
					counts[row.Label] = row.Requests
				}
				if counts["Default"] != 3 || counts["Pro"] != 2 {
					t.Errorf("requests by group = %v, want Default 3 and Pro 2", counts)
				}
			})

			t.Run("the cross-tabulation holds only the keys it was given", func(t *testing.T) {
				ada, bo := fixture.people["ada"].ID, fixture.people["bob"].ID
				cells, err := store.Cross(ctx, "user", "model", []string{ada, bo}, []string{"m-opus"}, Filter{})
				if err != nil {
					t.Fatal(err)
				}
				if len(cells) != 2 {
					t.Fatalf("got %d cells, want ada and bo on opus: %+v", len(cells), cells)
				}
				for _, cell := range cells {
					if cell.Col != "m-opus" || cell.Requests != 1 {
						t.Errorf("unexpected cell %+v", cell)
					}
				}

				// The window's argument comes before the keys' in the statement,
				// which is where an ordering mistake would put one in the other's
				// place.
				later, err := store.Cross(ctx, "user", "model", []string{ada, bo}, []string{"m-opus", "m-gpt"}, Filter{Since: thursday + hourMS})
				if err != nil {
					t.Fatal(err)
				}
				seen := map[[2]string]int64{}
				for _, cell := range later {
					seen[[2]string{cell.Row, cell.Col}] = cell.Requests
				}
				want := map[[2]string]int64{{ada, "m-gpt"}: 1, {bo, "m-opus"}: 1}
				if len(seen) != len(want) || seen[[2]string{ada, "m-gpt"}] != 1 || seen[[2]string{bo, "m-opus"}] != 1 {
					t.Errorf("cells after the first hour = %v, want %v", seen, want)
				}

				if none, err := store.Cross(ctx, "user", "model", nil, []string{"m-opus"}, Filter{}); err != nil || len(none) != 0 {
					t.Errorf("no rows asked for: %d cells, %v", len(none), err)
				}
				if _, err := store.Cross(ctx, "user", "user", []string{ada}, []string{bo}, Filter{}); err == nil {
					t.Error("a dimension crossed with itself should be refused")
				}
				if _, err := store.Cross(ctx, "user", "conversation", []string{ada}, []string{"x"}, Filter{}); err == nil {
					t.Error("an unknown dimension should be refused")
				}
			})

			t.Run("the heatmap is on the reader's clock", func(t *testing.T) {
				slots := func(offset time.Duration) map[[2]int]int64 {
					t.Helper()
					found, err := store.Heatmap(ctx, Filter{}, offset)
					if err != nil {
						t.Fatal(err)
					}
					out := map[[2]int]int64{}
					for _, slot := range found {
						out[[2]int{slot.Weekday, slot.Hour}] += slot.Requests
					}
					return out
				}

				utc := slots(0)
				for _, want := range [][2]int{{4, 0}, {4, 1}, {4, 2}, {4, 3}, {5, 1}} {
					if utc[want] != 1 {
						t.Errorf("UTC: slot %v has %d, want 1 (all: %v)", want, utc[want], utc)
					}
				}
				// An hour behind Greenwich, the first turn happened on Wednesday.
				if west := slots(-time.Hour); west[[2]int{3, 23}] != 1 || west[[2]int{5, 0}] != 1 {
					t.Errorf("UTC-1: %v, want Wednesday 23:00 and Friday 00:00 filled", west)
				}
				if east := slots(8 * time.Hour); east[[2]int{4, 8}] != 1 {
					t.Errorf("UTC+8: %v, want Thursday 08:00 filled", east)
				}
			})

			t.Run("a day bucket starts at the reader's midnight", func(t *testing.T) {
				day := 24 * time.Hour
				starts := func(offset time.Duration) []int64 {
					t.Helper()
					points, err := store.Series(ctx, Filter{}, day, offset)
					if err != nil {
						t.Fatal(err)
					}
					out := make([]int64, 0, len(points))
					for _, point := range points {
						out = append(out, point.At)
					}
					return out
				}
				midnight := thursday - 30*60_000

				if got := starts(0); len(got) != 2 || got[0] != midnight || got[1] != midnight+24*hourMS {
					t.Errorf("UTC day starts = %v, want Thursday and Friday midnight", got)
				}
				// Beijing's Thursday began at four in the afternoon on Wednesday,
				// Greenwich time, and the first four turns all fall inside it.
				if got := starts(8 * time.Hour); len(got) != 2 || got[0] != midnight-8*hourMS || got[1] != midnight+16*hourMS {
					t.Errorf("UTC+8 day starts = %v, want %d and %d", got, midnight-8*hourMS, midnight+16*hourMS)
				}
				if got := starts(-time.Hour); len(got) != 3 || got[0] != midnight-23*hourMS {
					t.Errorf("UTC-1 day starts = %v, want three days beginning at %d", got, midnight-23*hourMS)
				}
			})
		})
	}
}
