package notify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/auth"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/config"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/database"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/group"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

func fixture(t *testing.T) (*Store, *user.Store) {
	t.Helper()
	ctx := context.Background()

	db, err := database.Open(ctx, config.Database{
		Driver:       "sqlite",
		DSN:          filepath.Join(t.TempDir(), "notify.db"),
		MaxOpenConns: 8,
		MaxIdleConns: 4,
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	groups := group.NewStore(db)
	if _, err := groups.Create(ctx, nil, group.CreateInput{Name: "Default", IsDefault: true}); err != nil {
		t.Fatalf("create group: %v", err)
	}
	return NewStore(db), user.NewStore(db)
}

// The three audiences, and the one rule that cuts across all of them: an
// account is never surprised by something made before it existed.
func TestVisibilityRules(t *testing.T) {
	store, users := fixture(t)
	ctx := context.Background()

	owner, err := users.Create(ctx, nil, user.CreateInput{Username: "owner", PasswordHash: "x"})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}

	pushes := []Notification{
		{Audience: AudienceUser, UserID: owner.ID, Kind: "user_notice", CreatedAt: 1000},
		{Audience: AudienceAll, Kind: "all_notice", CreatedAt: 2000},
		{Audience: AudienceAdmins, Permission: "", Kind: "admin_any", CreatedAt: 3000},
		{Audience: AudienceAdmins, Permission: "feedback", Kind: "admin_feedback", CreatedAt: 4000},
		{Audience: AudienceAdmins, Permission: "security", Kind: "admin_security", CreatedAt: 5000},
	}
	for _, n := range pushes {
		if err := store.Push(ctx, nil, n); err != nil {
			t.Fatalf("push %s: %v", n.Kind, err)
		}
	}

	kindsOf := func(records []Notification) []string {
		out := make([]string, len(records))
		for i, r := range records {
			out[i] = r.Kind
		}
		return out
	}

	cases := []struct {
		name    string
		account user.User
		want    []string // in any order; checked as a set
	}{
		{
			name:    "the owner sees their own and everyone's",
			account: user.User{ID: owner.ID, CreatedAt: 0},
			want:    []string{"user_notice", "all_notice"},
		},
		{
			name:    "a stranger who existed in time sees only the broadcast",
			account: user.User{ID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", CreatedAt: 1500},
			want:    []string{"all_notice"},
		},
		{
			name:    "an account created after the broadcast never sees it",
			account: user.User{ID: "01ARZ3NDEKTSV4RRFFQ69G5FAB", CreatedAt: 2500},
			want:    nil,
		},
		{
			name: "an administrator with one grant sees the unrestricted notice and their own",
			account: user.User{
				ID: "admin1", Role: user.RoleAdmin, AdminPermissions: []string{"feedback"}, CreatedAt: 0,
			},
			want: []string{"all_notice", "admin_any", "admin_feedback"},
		},
		{
			name:    "a super admin sees every administrative notice regardless of permission",
			account: user.User{ID: "admin2", Role: user.RoleSuperAdmin, CreatedAt: 0},
			want:    []string{"all_notice", "admin_any", "admin_feedback", "admin_security"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			records, err := store.List(ctx, tc.account, 100, 0)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			got := kindsOf(records)
			if len(got) != len(tc.want) {
				t.Fatalf("kinds = %v, want %v", got, tc.want)
			}
			for _, wantKind := range tc.want {
				found := false
				for _, k := range got {
					if k == wantKind {
						found = true
					}
				}
				if !found {
					t.Errorf("kinds = %v, missing %q", got, wantKind)
				}
			}
		})
	}
}

func TestPushRefusesWhatItCannotAddressOrWord(t *testing.T) {
	store, _ := fixture(t)
	ctx := context.Background()

	cases := map[string]struct {
		in   Notification
		want error
	}{
		"unknown audience": {Notification{Audience: "everyone", Kind: "x"}, ErrInvalidAudience},
		"user with no id":  {Notification{Audience: AudienceUser, Kind: "x"}, ErrUserRequired},
		"unknown grant":    {Notification{Audience: AudienceAdmins, Permission: "nonsense", Kind: "x"}, ErrInvalidPermission},
		"no kind":          {Notification{Audience: AudienceAll}, ErrKindRequired},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := store.Push(ctx, nil, tc.in); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// The whole reason Push takes a Queryer: a producer that pushes inside the
// same transaction as the write it is about must see the notice vanish with
// everything else when that transaction rolls back.
func TestPushInsideATransactionRollsBackWithIt(t *testing.T) {
	store, _ := fixture(t)
	ctx := context.Background()

	sentinel := errors.New("the triggering write failed after all")
	err := store.db.Tx(ctx, func(tx *database.Tx) error {
		if err := store.Push(ctx, tx, Notification{Audience: AudienceAll, Kind: "x", CreatedAt: 1}); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("tx err = %v, want the sentinel", err)
	}

	records, err := store.List(ctx, user.User{CreatedAt: 0}, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("%d notifications survived a rolled-back transaction", len(records))
	}
}

func TestPushInsideATransactionCommitsWithIt(t *testing.T) {
	store, _ := fixture(t)
	ctx := context.Background()

	err := store.db.Tx(ctx, func(tx *database.Tx) error {
		return store.Push(ctx, tx, Notification{
			Audience: AudienceAll, Kind: "announcement", Params: map[string]any{"title": "Maintenance"}, CreatedAt: 1,
		})
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}

	records, err := store.List(ctx, user.User{CreatedAt: 0}, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 1 || records[0].Kind != "announcement" {
		t.Fatalf("records = %+v, want the one committed notice", records)
	}
	if records[0].Params["title"] != "Maintenance" {
		t.Errorf("params = %+v, want the title carried through", records[0].Params)
	}
}

func TestMarkReadMovesTheWatermarkForwardNeverBack(t *testing.T) {
	store, users := fixture(t)
	ctx := context.Background()
	account, err := users.Create(ctx, nil, user.CreateInput{Username: "watermark", PasswordHash: "x"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := account.ID

	if seenAt, err := store.SeenAt(ctx, userID); err != nil || seenAt != 0 {
		t.Fatalf("seen at for an account that never opened the bell = %d, %v; want 0, nil", seenAt, err)
	}

	if err := store.MarkRead(ctx, userID, 5000); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if seenAt, _ := store.SeenAt(ctx, userID); seenAt != 5000 {
		t.Errorf("seen at = %d, want 5000", seenAt)
	}

	// An older watermark arriving later — two tabs racing to clear the bell —
	// must not undo the more recent one.
	if err := store.MarkRead(ctx, userID, 1000); err != nil {
		t.Fatalf("mark read (older): %v", err)
	}
	if seenAt, _ := store.SeenAt(ctx, userID); seenAt != 5000 {
		t.Errorf("seen at moved backwards to %d", seenAt)
	}

	if err := store.MarkRead(ctx, userID, 0); err != nil {
		t.Fatalf("mark read (now): %v", err)
	}
	if seenAt, _ := store.SeenAt(ctx, userID); seenAt <= 5000 {
		t.Errorf("seen at = %d after marking read with no explicit moment, want it to have moved to now", seenAt)
	}

	// A watermark ahead of the server's own clock is not something a client
	// gets to set: an up_to in the future clamps to now instead of parking
	// there and swallowing whatever is pushed before real time catches up.
	beforeCall := time.Now().UnixMilli()
	if err := store.MarkRead(ctx, userID, beforeCall+time.Hour.Milliseconds()); err != nil {
		t.Fatalf("mark read (future): %v", err)
	}
	if seenAt, _ := store.SeenAt(ctx, userID); seenAt > time.Now().UnixMilli() || seenAt < beforeCall {
		t.Errorf("seen at = %d, want it clamped to server now (around %d) rather than parked an hour ahead", seenAt, beforeCall)
	}
}

func TestPollIsAscendingAndUnreadTracksTheWatermark(t *testing.T) {
	store, users := fixture(t)
	ctx := context.Background()

	owner, err := users.Create(ctx, nil, user.CreateInput{Username: "reader", PasswordHash: "x"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	// Spaced well beyond the poll grace window (see pollGraceMillis), so this
	// test exercises ordering and the watermark without also exercising the
	// window itself — that has its own test.
	for i, at := range []int64{1000, 20000, 30000} {
		if err := store.Push(ctx, nil, Notification{
			Audience: AudienceUser, UserID: owner.ID, Kind: "k", CreatedAt: at,
		}); err != nil {
			t.Fatalf("push %d: %v", i, err)
		}
	}
	account := user.User{ID: owner.ID, CreatedAt: 0}

	polled, err := store.Poll(ctx, account, 15000)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(polled) != 2 || polled[0].CreatedAt != 20000 || polled[1].CreatedAt != 30000 {
		t.Fatalf("polled = %+v, want the 20000 and 30000 rows, oldest first", polled)
	}

	if unread, err := store.Unread(ctx, account, 0); err != nil || unread != 3 {
		t.Fatalf("unread from zero = %d, %v; want 3, nil", unread, err)
	}
	if err := store.MarkRead(ctx, account.ID, 20000); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	seenAt, err := store.SeenAt(ctx, account.ID)
	if err != nil {
		t.Fatalf("seen at: %v", err)
	}
	if unread, err := store.Unread(ctx, account, seenAt); err != nil || unread != 1 {
		t.Fatalf("unread after marking through 20000 = %d, %v; want 1, nil", unread, err)
	}
}

// Old rows are pruned; the watermark that counted them is not — it stays
// correct for whatever arrives next.
func TestPruneDropsOldRowsAndLeavesTheWatermark(t *testing.T) {
	store, users := fixture(t)
	ctx := context.Background()
	account, err := users.Create(ctx, nil, user.CreateInput{Username: "prunee", PasswordHash: "x"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := account.ID

	if err := store.Push(ctx, nil, Notification{
		Audience: AudienceUser, UserID: userID, Kind: "old", CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("push old: %v", err)
	}
	if err := store.MarkRead(ctx, userID, 999); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	removed, err := store.Prune(ctx, time.UnixMilli(50000))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 1 {
		t.Errorf("pruned %d rows, want 1", removed)
	}
	records, err := store.List(ctx, user.User{ID: userID, CreatedAt: 0}, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("%d rows survived pruning", len(records))
	}
	if seenAt, err := store.SeenAt(ctx, userID); err != nil || seenAt != 999 {
		t.Errorf("seen at after pruning = %d, %v; want 999, nil — pruning must not touch the watermark", seenAt, err)
	}
}

// Retract is what a source removes its own notices with when it stops
// standing behind them — an announcement unpublished or deleted, in
// particular. It matches on kind and ref together, so retracting one source's
// notices can never touch another kind's rows that happen to reuse the same
// id space, and a blank ref — every kind that never sets one — retracts
// nothing rather than deleting every row of that kind ever pushed.
func TestRetractRemovesOnlyTheMatchingKindAndRef(t *testing.T) {
	store, _ := fixture(t)
	ctx := context.Background()
	everyone := user.User{CreatedAt: 0}

	if err := store.Push(ctx, nil, Notification{
		Audience: AudienceAll, Kind: "announcement", Ref: "ann-1", CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("push ann-1: %v", err)
	}
	if err := store.Push(ctx, nil, Notification{
		Audience: AudienceAll, Kind: "announcement", Ref: "ann-2", CreatedAt: 2000,
	}); err != nil {
		t.Fatalf("push ann-2: %v", err)
	}
	// A different kind, same ref value, and a notice with no ref at all —
	// neither should be touched by retracting "announcement"/"ann-1".
	if err := store.Push(ctx, nil, Notification{
		Audience: AudienceAll, Kind: "other", Ref: "ann-1", CreatedAt: 3000,
	}); err != nil {
		t.Fatalf("push other: %v", err)
	}
	if err := store.Push(ctx, nil, Notification{Audience: AudienceAll, Kind: "no_ref", CreatedAt: 4000}); err != nil {
		t.Fatalf("push no_ref: %v", err)
	}

	removed, err := store.Retract(ctx, nil, "announcement", "ann-1")
	if err != nil {
		t.Fatalf("retract: %v", err)
	}
	if removed != 1 {
		t.Fatalf("retract removed %d rows, want 1", removed)
	}

	remaining, err := store.List(ctx, everyone, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(remaining) != 3 {
		t.Fatalf("remaining = %+v, want the other three rows untouched", remaining)
	}
	announcements := 0
	for _, n := range remaining {
		if n.Kind == "announcement" {
			announcements++
		}
	}
	if announcements != 1 {
		t.Errorf("%d announcement notices remain, want only ann-2's", announcements)
	}

	// A blank ref is not "everything of this kind": it matches nothing.
	if removed, err := store.Retract(ctx, nil, "no_ref", ""); err != nil || removed != 0 {
		t.Fatalf("retract with a blank ref removed %d rows (err %v), want 0", removed, err)
	}
	if remaining, err := store.List(ctx, everyone, 10, 0); err != nil || len(remaining) != 3 {
		t.Fatalf("remaining after a blank-ref retract = %+v (err %v), want still three", remaining, err)
	}
}

// The grace window's whole reason to exist: a row stamped before the client's
// cursor, but whose own write lands after a row the client has already
// polled, must not be lost the moment an exact "> after" would skip it.
func TestPollReturnsALateRowStampedBeforeTheCursorWithinTheGraceWindow(t *testing.T) {
	store, users := fixture(t)
	ctx := context.Background()
	owner, err := users.Create(ctx, nil, user.CreateInput{Username: "poller", PasswordHash: "x"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	account := user.User{ID: owner.ID, CreatedAt: 0}

	// The "later" row: pushed (and committed) first, and what a client polls
	// and advances its cursor to.
	if err := store.Push(ctx, nil, Notification{
		Audience: AudienceUser, UserID: owner.ID, Kind: "later", CreatedAt: 5000,
	}); err != nil {
		t.Fatalf("push later: %v", err)
	}
	first, err := store.Poll(ctx, account, 0)
	if err != nil || len(first) != 1 || first[0].Kind != "later" {
		t.Fatalf("first poll = %+v (err %v), want just the later row", first, err)
	}
	cursor := first[0].CreatedAt

	// The "earlier" row: stamped before the cursor already advanced past, but
	// its own write only lands now — a transaction that started before
	// "later" but committed after it.
	if err := store.Push(ctx, nil, Notification{
		Audience: AudienceUser, UserID: owner.ID, Kind: "earlier", CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("push earlier: %v", err)
	}

	// A poll asking after exactly the later row's own created_at still
	// catches the earlier one, ordered oldest first: an exact "> cursor"
	// would never see 1000 again once the cursor sits at 5000.
	second, err := store.Poll(ctx, account, cursor)
	if err != nil {
		t.Fatalf("poll again: %v", err)
	}
	if len(second) != 2 || second[0].Kind != "earlier" || second[1].Kind != "later" {
		t.Fatalf("poll after the cursor = %+v, want both rows, oldest first, "+
			"the grace window catching the late-committed one", second)
	}

	// Outside the window, the earlier row is gone from a poll for good —
	// exactly the tradeoff the grace window makes, in exchange for the client
	// de-duplicating by id instead.
	if beyond, err := store.Poll(ctx, account, cursor+pollGraceMillis+1); err != nil || len(beyond) != 0 {
		t.Fatalf("poll past the grace window = %+v (err %v), want nothing left to catch", beyond, err)
	}
}

// The page size a single poll answer is held to, regardless of how large the
// backlog behind it is.
func TestPollIsCappedAtItsLimit(t *testing.T) {
	store, users := fixture(t)
	ctx := context.Background()
	owner, err := users.Create(ctx, nil, user.CreateInput{Username: "flood", PasswordHash: "x"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	for i := 0; i < pollLimit+50; i++ {
		if err := store.Push(ctx, nil, Notification{
			Audience: AudienceUser, UserID: owner.ID, Kind: "k", CreatedAt: int64(1000 + i),
		}); err != nil {
			t.Fatalf("push %d: %v", i, err)
		}
	}

	polled, err := store.Poll(ctx, user.User{ID: owner.ID, CreatedAt: 0}, 0)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(polled) != pollLimit {
		t.Fatalf("polled %d rows, want the %d cap", len(polled), pollLimit)
	}
	if polled[0].CreatedAt != 1000 || polled[len(polled)-1].CreatedAt != int64(1000+pollLimit-1) {
		t.Fatalf("polled range = [%d, %d], want the oldest %d rows in order",
			polled[0].CreatedAt, polled[len(polled)-1].CreatedAt, pollLimit)
	}
}

// The bell's badge and the announcement bell's own badge must never both
// claim credit for the same notice: an announcement counts nowhere near this
// one, even though it still shows up in the list and the poll a toast is
// raised from.
func TestUnreadExcludesAnnouncementsButListAndPollStillReturnThem(t *testing.T) {
	store, users := fixture(t)
	ctx := context.Background()
	owner, err := users.Create(ctx, nil, user.CreateInput{Username: "reader", PasswordHash: "x"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.Push(ctx, nil, Notification{
		Audience: AudienceAll, Kind: "announcement",
		Params: map[string]any{"title": "Maintenance"}, CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("push announcement: %v", err)
	}
	if err := store.Push(ctx, nil, Notification{
		Audience: AudienceUser, UserID: owner.ID, Kind: "cards_granted", CreatedAt: 2000,
	}); err != nil {
		t.Fatalf("push notice: %v", err)
	}
	account := user.User{ID: owner.ID, CreatedAt: 0}

	if unread, err := store.Unread(ctx, account, 0); err != nil || unread != 1 {
		t.Fatalf("unread = %d, %v; want 1 — the announcement excluded", unread, err)
	}
	if listed, err := store.List(ctx, account, 10, 0); err != nil || len(listed) != 2 {
		t.Fatalf("list = %+v (err %v), want both rows, the announcement included", listed, err)
	}
	if polled, err := store.Poll(ctx, account, 0); err != nil || len(polled) != 2 {
		t.Fatalf("poll = %+v (err %v), want both rows, the announcement included", polled, err)
	}
}

func TestHTTPListPollAndRead(t *testing.T) {
	store, users := fixture(t)
	ctx := context.Background()

	account, err := users.Create(ctx, nil, user.CreateInput{Username: "reader", PasswordHash: "x"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.Push(ctx, nil, Notification{
		Audience: AudienceUser, UserID: account.ID, Kind: "cards_granted",
		Params: map[string]any{"count": float64(3)}, Link: "/usage", CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("push: %v", err)
	}

	mux := http.NewServeMux()
	NewHandlers(store).Routes(mux)
	reqCtx := auth.WithUser(context.Background(), account)

	get := func(path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(reqCtx)
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}

	type feed struct {
		Notifications []Notification `json:"notifications"`
		Unread        int            `json:"unread"`
		SeenAt        int64          `json:"seen_at"`
		Now           int64          `json:"now"`
	}

	before := time.Now().UnixMilli()
	listResp := get("/api/notifications")
	if listResp.Code != http.StatusOK {
		t.Fatalf("list = %d %s", listResp.Code, listResp.Body.String())
	}
	var listed feed
	if err := json.Unmarshal(listResp.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Notifications) != 1 || listed.Unread != 1 || listed.SeenAt != 0 {
		t.Fatalf("list = %+v, want one unread notice and no watermark yet", listed)
	}
	if listed.Notifications[0].Link != "/usage" || listed.Notifications[0].Params["count"] != float64(3) {
		t.Errorf("notification = %+v, want the link and params round-tripped", listed.Notifications[0])
	}
	if listed.Now < before {
		t.Errorf("list now = %d, want the server's own clock at call time (>= %d)", listed.Now, before)
	}

	polledResp := get("/api/notifications/poll?after=0")
	if polledResp.Code != http.StatusOK {
		t.Fatalf("poll = %d %s", polledResp.Code, polledResp.Body.String())
	}
	var polled feed
	if err := json.Unmarshal(polledResp.Body.Bytes(), &polled); err != nil {
		t.Fatalf("decode poll: %v", err)
	}
	if len(polled.Notifications) != 1 || polled.Unread != 1 {
		t.Fatalf("poll = %+v, want the one row still unread", polled)
	}
	if polled.Now < before {
		t.Errorf("poll now = %d, want the server's own clock at call time (>= %d)", polled.Now, before)
	}

	// The body uses up_to, not until — the client's own name for the shared
	// protocol field, checked here so the two sides cannot drift apart.
	readRequest := httptest.NewRequest(http.MethodPost, "/api/notifications/read", strings.NewReader(`{"up_to": 1000}`)).
		WithContext(reqCtx)
	readRequest.Header.Set("Content-Type", "application/json")
	readResp := httptest.NewRecorder()
	mux.ServeHTTP(readResp, readRequest)
	if readResp.Code != http.StatusNoContent {
		t.Fatalf("read = %d %s", readResp.Code, readResp.Body.String())
	}

	afterRead := get("/api/notifications/poll?after=0")
	var afterFeed feed
	if err := json.Unmarshal(afterRead.Body.Bytes(), &afterFeed); err != nil {
		t.Fatalf("decode poll after read: %v", err)
	}
	if afterFeed.Unread != 0 {
		t.Errorf("unread after marking read = %d, want 0", afterFeed.Unread)
	}
	// The row itself still comes back from a poll asking since the beginning
	// of time — "mark read" moves the badge, not history.
	if len(afterFeed.Notifications) != 1 {
		t.Errorf("poll since 0 after marking read returned %d rows, want the row still there", len(afterFeed.Notifications))
	}
}
