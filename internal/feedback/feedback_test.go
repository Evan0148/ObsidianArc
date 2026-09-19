package feedback

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/config"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/database"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/group"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

func fixture(t *testing.T) (*Store, user.User, user.User) {
	t.Helper()
	ctx := context.Background()

	db, err := database.Open(ctx, config.Database{
		Driver:       "sqlite",
		DSN:          filepath.Join(t.TempDir(), "feedback.db"),
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
	users := user.NewStore(db)
	author, err := users.Create(ctx, nil, user.CreateInput{Username: "author", PasswordHash: "x"})
	if err != nil {
		t.Fatalf("create author: %v", err)
	}
	other, err := users.Create(ctx, nil, user.CreateInput{Username: "other", PasswordHash: "x"})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	return NewStore(db), author, other
}

func report(kind Kind, priority Priority, title string) Input {
	return Input{Kind: kind, Priority: priority, Title: title, Body: "what happened, at length"}
}

func TestCreateStoresWhatWasWrittenAndOpensIt(t *testing.T) {
	store, author, _ := fixture(t)
	ctx := context.Background()

	record, err := store.Create(ctx, author.ID, report(KindBug, PriorityHigh, "  Upload fails  "))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if record.Title != "Upload fails" {
		t.Errorf("title = %q, want the trimmed text", record.Title)
	}
	if record.Status != StatusOpen {
		t.Errorf("status = %q, want a new report to be open", record.Status)
	}
	if record.UserID != author.ID {
		t.Errorf("user_id = %q, want the author's", record.UserID)
	}
}

func TestCreateDefaultsAndRefusals(t *testing.T) {
	store, author, _ := fixture(t)
	ctx := context.Background()

	// An omitted kind and priority are the two safest answers rather than a
	// refusal: the form always sends them, and a client that does not is
	// reporting a bug, not filing a malformed one.
	defaulted, err := store.Create(ctx, author.ID, Input{Title: "No enums", Body: "…"})
	if err != nil {
		t.Fatalf("create with defaults: %v", err)
	}
	if defaulted.Kind != KindBug || defaulted.Priority != PriorityMedium {
		t.Errorf("defaults = %q/%q, want bug/medium", defaulted.Kind, defaulted.Priority)
	}

	cases := map[string]struct {
		in   Input
		want error
	}{
		"no title":       {Input{Title: "   ", Body: "x"}, ErrInvalidTitle},
		"long title":     {Input{Title: strings.Repeat("x", MaxTitleChars+1), Body: "x"}, ErrInvalidTitle},
		"no body":        {Input{Title: "t", Body: " "}, ErrInvalidBody},
		"long body":      {Input{Title: "t", Body: strings.Repeat("x", MaxBodyChars+1)}, ErrInvalidBody},
		"unknown kind":   {Input{Title: "t", Body: "b", Kind: "question"}, ErrInvalidKind},
		"unknown weight": {Input{Title: "t", Body: "b", Priority: "urgent"}, ErrInvalidPrio},
	}
	for name, testcase := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := store.Create(ctx, author.ID, testcase.in); !errors.Is(err, testcase.want) {
				t.Errorf("err = %v, want %v", err, testcase.want)
			}
		})
	}
}

// The daily cap is a check followed by a write, which is the shape this
// repository requires a database lock for rather than a mutex. Real
// goroutines, because a count and a standalone insert let every one of them
// observe the same last free slot and take it.
func TestTheDailyCapHoldsUnderConcurrentSends(t *testing.T) {
	store, author, _ := fixture(t)
	ctx := context.Background()

	// One short of the cap, so every writer below is racing for the same
	// single remaining slot.
	for i := 0; i < MaxPerDay-1; i++ {
		if _, err := store.Create(ctx, author.ID, report(KindBug, PriorityLow, "filler")); err != nil {
			t.Fatalf("filler %d: %v", i, err)
		}
	}

	const writers = 8
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		accepted int
		refused  int
		other    []error
	)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Create(ctx, author.ID, report(KindBug, PriorityLow, "racer"))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				accepted++
			case errors.Is(err, ErrTooMany):
				refused++
			default:
				other = append(other, err)
			}
		}()
	}
	wg.Wait()

	if len(other) > 0 {
		t.Fatalf("unexpected error: %v", other[0])
	}
	if accepted != 1 {
		t.Errorf("%d of %d racers took the last slot, want exactly 1", accepted, writers)
	}
	if refused != writers-1 {
		t.Errorf("%d refusals, want %d", refused, writers-1)
	}

	records, _, err := store.List(ctx, nil, Filter{UserID: author.ID, Limit: 100})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != MaxPerDay {
		t.Errorf("%d rows stored, want the cap of %d", len(records), MaxPerDay)
	}
}

// The cap belongs to one account, not to the instance: two people reporting
// the same outage on the same morning are not each other's problem.
func TestTheCapIsPerAccount(t *testing.T) {
	store, author, other := fixture(t)
	ctx := context.Background()

	for i := 0; i < MaxPerDay; i++ {
		if _, err := store.Create(ctx, author.ID, report(KindBug, PriorityLow, "theirs")); err != nil {
			t.Fatalf("filler %d: %v", i, err)
		}
	}
	if _, err := store.Create(ctx, author.ID, report(KindBug, PriorityLow, "one too many")); !errors.Is(err, ErrTooMany) {
		t.Fatalf("err = %v, want ErrTooMany", err)
	}
	if _, err := store.Create(ctx, other.ID, report(KindIdea, PriorityLow, "mine")); err != nil {
		t.Errorf("a second account was refused because the first had been busy: %v", err)
	}
}

func TestListFiltersAndCounts(t *testing.T) {
	store, author, other := fixture(t)
	ctx := context.Background()

	bug, err := store.Create(ctx, author.ID, report(KindBug, PriorityHigh, "Streaming stops"))
	if err != nil {
		t.Fatalf("create bug: %v", err)
	}
	if _, err := store.Create(ctx, other.ID, report(KindIdea, PriorityLow, "A darker theme")); err != nil {
		t.Fatalf("create idea: %v", err)
	}

	byKind, total, err := store.List(ctx, nil, Filter{Kind: KindIdea})
	if err != nil {
		t.Fatalf("list by kind: %v", err)
	}
	if total != 1 || len(byKind) != 1 || byKind[0].Kind != KindIdea {
		t.Errorf("kind filter returned %d rows (total %d), want the one idea", len(byKind), total)
	}
	// The author comes from the join, and it is the whole reason the
	// operator's list is worth reading.
	if byKind[0].Username != "other" {
		t.Errorf("username = %q, want the author resolved", byKind[0].Username)
	}

	bySearch, _, err := store.List(ctx, nil, Filter{Search: "STREAMING"})
	if err != nil {
		t.Fatalf("list by search: %v", err)
	}
	if len(bySearch) != 1 || bySearch[0].ID != bug.ID {
		t.Errorf("search is case sensitive: got %d rows", len(bySearch))
	}

	summary, err := store.Counts(ctx, nil)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if summary.Total != 2 || summary.Open != 2 || summary.Bugs != 1 || summary.Ideas != 1 || summary.HighOpen != 1 {
		t.Errorf("summary = %+v, want 2/2/1/1/1", summary)
	}
}

// Counts runs against an empty table on every fresh instance, and SUM over
// no rows is NULL on both engines.
func TestCountsOnAnEmptyTable(t *testing.T) {
	store, _, _ := fixture(t)

	summary, err := store.Counts(context.Background(), nil)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if summary != (Summary{}) {
		t.Errorf("summary = %+v, want every count zero", summary)
	}
}

func TestStatusAndDelete(t *testing.T) {
	store, author, _ := fixture(t)
	ctx := context.Background()

	record, err := store.Create(ctx, author.ID, report(KindBug, PriorityMedium, "Something"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	resolved, err := store.SetStatus(ctx, record.ID, StatusResolved)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Status != StatusResolved {
		t.Errorf("status = %q, want resolved", resolved.Status)
	}
	// The report is what somebody wrote, and resolving does not edit it.
	if resolved.Title != record.Title || resolved.Body != record.Body {
		t.Error("resolving rewrote the report")
	}

	if _, err := store.SetStatus(ctx, record.ID, "wontfix"); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("err = %v, want ErrInvalidStatus", err)
	}
	if _, err := store.SetStatus(ctx, "01ARZ3NDEKTSV4RRFFQ69G5FAV", StatusOpen); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}

	if err := store.Delete(ctx, record.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := store.Delete(ctx, record.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete err = %v, want ErrNotFound", err)
	}
	if _, err := store.ByID(ctx, nil, record.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("by id after delete = %v, want ErrNotFound", err)
	}
}

func TestListForIsTheAuthorsOwn(t *testing.T) {
	store, author, other := fixture(t)
	ctx := context.Background()

	if _, err := store.Create(ctx, author.ID, report(KindBug, PriorityLow, "mine")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.Create(ctx, other.ID, report(KindBug, PriorityLow, "theirs")); err != nil {
		t.Fatalf("create: %v", err)
	}

	records, err := store.ListFor(ctx, nil, author.ID)
	if err != nil {
		t.Fatalf("list for author: %v", err)
	}
	if len(records) != 1 || records[0].Title != "mine" {
		t.Errorf("an author was shown %d reports, want only their own", len(records))
	}
}

// A deleted account takes its reports with it, which is the foreign key's
// job rather than a sweep somebody has to remember to run.
func TestReportsGoWithTheAccount(t *testing.T) {
	store, author, _ := fixture(t)
	ctx := context.Background()

	if _, err := store.Create(ctx, author.ID, report(KindBug, PriorityLow, "mine")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.db.Exec(ctx, `DELETE FROM users WHERE id = ?`, author.ID); err != nil {
		t.Fatalf("delete account: %v", err)
	}

	_, total, err := store.List(ctx, nil, Filter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 {
		t.Errorf("%d reports outlived their author", total)
	}
}

// --- the conversation ---------------------------------------------------------

func TestRepliesMoveTheUnreadFlagToTheOtherSide(t *testing.T) {
	store, author, staff := fixture(t)
	ctx := context.Background()

	record, err := store.Create(ctx, author.ID, report(KindBug, PriorityMedium, "Something"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// A fresh report is already waiting for the operator, and its author has
	// obviously read their own words.
	fresh, err := store.ByID(ctx, nil, record.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if fresh.AuthorUnread {
		t.Error("a new report is unread for the person who just wrote it")
	}

	if _, err := store.AddReply(ctx, ReplyInput{
		FeedbackID: record.ID, UserID: staff.ID, FromStaff: true, Body: "Which model?",
	}); err != nil {
		t.Fatalf("staff reply: %v", err)
	}
	after, err := store.ByID(ctx, nil, record.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if !after.AuthorUnread || after.OperatorUnread {
		t.Errorf("after a staff reply: author_unread=%v operator_unread=%v, want true/false",
			after.AuthorUnread, after.OperatorUnread)
	}
	if after.Replies != 1 {
		t.Errorf("replies = %d, want 1", after.Replies)
	}

	if _, err := store.AddReply(ctx, ReplyInput{
		FeedbackID: record.ID, UserID: author.ID, Body: "The fast one.", RequireOwner: true,
	}); err != nil {
		t.Fatalf("author reply: %v", err)
	}
	back, err := store.ByID(ctx, nil, record.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if back.AuthorUnread || !back.OperatorUnread {
		t.Errorf("after the author answers: author_unread=%v operator_unread=%v, want false/true",
			back.AuthorUnread, back.OperatorUnread)
	}

	// Oldest first: a conversation only reads in the order it was said.
	replies, err := store.Replies(ctx, nil, record.ID)
	if err != nil {
		t.Fatalf("replies: %v", err)
	}
	if len(replies) != 2 || replies[0].Body != "Which model?" || !replies[0].FromStaff || replies[1].FromStaff {
		t.Errorf("thread = %+v, want the staff question then the author's answer", replies)
	}
	if replies[0].Username != "other" {
		t.Errorf("reply author = %q, want it resolved from the join", replies[0].Username)
	}
}

// The author's own endpoint passes RequireOwner, and the store is where that
// is enforced — not in a check above it that a second caller could forget.
func TestAnAuthorCannotReplyToSomebodyElsesThread(t *testing.T) {
	store, author, stranger := fixture(t)
	ctx := context.Background()

	record, err := store.Create(ctx, author.ID, report(KindBug, PriorityLow, "Mine"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = store.AddReply(ctx, ReplyInput{
		FeedbackID: record.ID, UserID: stranger.ID, Body: "hello", RequireOwner: true,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound — absent, not forbidden", err)
	}
	// An operator, on the other hand, answers other people's threads for a
	// living, and says so by not asking for ownership.
	if _, err := store.AddReply(ctx, ReplyInput{
		FeedbackID: record.ID, UserID: stranger.ID, FromStaff: true, Body: "looking into it",
	}); err != nil {
		t.Errorf("operator reply refused: %v", err)
	}
}

func TestThreadIsScopedToItsOwner(t *testing.T) {
	store, author, stranger := fixture(t)
	ctx := context.Background()

	record, err := store.Create(ctx, author.ID, report(KindBug, PriorityLow, "Mine"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.Thread(ctx, nil, record.ID, stranger.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("stranger's read = %v, want ErrNotFound", err)
	}
	thread, err := store.Thread(ctx, nil, record.ID, author.ID)
	if err != nil {
		t.Fatalf("author's read: %v", err)
	}
	if thread.Feedback.ID != record.ID || len(thread.Replies) != 0 {
		t.Errorf("thread = %+v, want the report and no replies yet", thread)
	}
	// The operator passes no owner at all.
	if _, err := store.Thread(ctx, nil, record.ID, ""); err != nil {
		t.Errorf("operator's read: %v", err)
	}
}

func TestMarkSeenAndUnreadCount(t *testing.T) {
	store, author, staff := fixture(t)
	ctx := context.Background()

	first, err := store.Create(ctx, author.ID, report(KindBug, PriorityLow, "One"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	second, err := store.Create(ctx, author.ID, report(KindIdea, PriorityLow, "Two"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, id := range []string{first.ID, second.ID} {
		if _, err := store.AddReply(ctx, ReplyInput{
			FeedbackID: id, UserID: staff.ID, FromStaff: true, Body: "answered",
		}); err != nil {
			t.Fatalf("reply: %v", err)
		}
	}

	count, err := store.UnreadFor(ctx, nil, author.ID)
	if err != nil {
		t.Fatalf("unread: %v", err)
	}
	if count != 2 {
		t.Errorf("unread = %d, want 2", count)
	}

	if err := store.MarkSeen(ctx, first.ID, false); err != nil {
		t.Fatalf("mark seen: %v", err)
	}
	if count, _ = store.UnreadFor(ctx, nil, author.ID); count != 1 {
		t.Errorf("unread after reading one = %d, want 1", count)
	}
	// Reading one side never clears the other's.
	row, err := store.ByID(ctx, nil, first.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if row.OperatorUnread {
		t.Error("the author's read cleared the operator's flag too")
	}
	// And it is not a change to the record: the operator's list shows that.
	if row.UpdatedAt != first.UpdatedAt && row.Replies == 0 {
		t.Error("reading a thread moved updated_at")
	}

	staffSide, err := store.Counts(ctx, nil)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if staffSide.Awaiting != 0 {
		t.Errorf("awaiting = %d, want 0 — the operator spoke last in both", staffSide.Awaiting)
	}
	if _, err := store.AddReply(ctx, ReplyInput{
		FeedbackID: first.ID, UserID: author.ID, Body: "still broken", RequireOwner: true,
	}); err != nil {
		t.Fatalf("author reply: %v", err)
	}
	if staffSide, _ = store.Counts(ctx, nil); staffSide.Awaiting != 1 {
		t.Errorf("awaiting = %d, want 1", staffSide.Awaiting)
	}
}

// The thread's length cap is the same check-then-write shape the daily cap
// is, held by the thread's own row rather than the author's — two people can
// be writing into one conversation at once, which is exactly the case a
// per-account lock would miss.
func TestTheThreadCapHoldsUnderConcurrentReplies(t *testing.T) {
	store, author, staff := fixture(t)
	ctx := context.Background()

	record, err := store.Create(ctx, author.ID, report(KindBug, PriorityLow, "Busy"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < MaxRepliesPerThread-1; i++ {
		if _, err := store.AddReply(ctx, ReplyInput{
			FeedbackID: record.ID, UserID: staff.ID, FromStaff: true, Body: "filler",
		}); err != nil {
			t.Fatalf("filler %d: %v", i, err)
		}
	}

	const writers = 8
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		accepted int
		refused  int
		other    []error
	)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		// Both sides at once, which is the shape a thread actually sees.
		staffTurn := i%2 == 0
		go func() {
			defer wg.Done()
			writer := author.ID
			if staffTurn {
				writer = staff.ID
			}
			_, err := store.AddReply(ctx, ReplyInput{
				FeedbackID: record.ID, UserID: writer, FromStaff: staffTurn, Body: "racer",
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				accepted++
			case errors.Is(err, ErrThreadFull):
				refused++
			default:
				other = append(other, err)
			}
		}()
	}
	wg.Wait()

	if len(other) > 0 {
		t.Fatalf("unexpected error: %v", other[0])
	}
	if accepted != 1 {
		t.Errorf("%d of %d racers took the last slot, want exactly 1", accepted, writers)
	}
	if refused != writers-1 {
		t.Errorf("%d refusals, want %d", refused, writers-1)
	}
	replies, err := store.Replies(ctx, nil, record.ID)
	if err != nil {
		t.Fatalf("replies: %v", err)
	}
	if len(replies) != MaxRepliesPerThread {
		t.Errorf("%d replies stored, want the cap of %d", len(replies), MaxRepliesPerThread)
	}
}

func TestDeletingOneReplyAndThenTheWholeThread(t *testing.T) {
	store, author, staff := fixture(t)
	ctx := context.Background()

	record, err := store.Create(ctx, author.ID, report(KindBug, PriorityLow, "Spammed"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	junk, err := store.AddReply(ctx, ReplyInput{
		FeedbackID: record.ID, UserID: staff.ID, FromStaff: true, Body: "buy my thing",
	})
	if err != nil {
		t.Fatalf("reply: %v", err)
	}
	kept, err := store.AddReply(ctx, ReplyInput{
		FeedbackID: record.ID, UserID: staff.ID, FromStaff: true, Body: "actually looking into it",
	})
	if err != nil {
		t.Fatalf("reply: %v", err)
	}

	// A reply id from another thread must not be enough to delete this one's.
	if err := store.DeleteReply(ctx, "01ARZ3NDEKTSV4RRFFQ69G5FAV", junk.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-thread delete = %v, want ErrNotFound", err)
	}
	if err := store.DeleteReply(ctx, record.ID, junk.ID); err != nil {
		t.Fatalf("delete reply: %v", err)
	}
	replies, err := store.Replies(ctx, nil, record.ID)
	if err != nil {
		t.Fatalf("replies: %v", err)
	}
	if len(replies) != 1 || replies[0].ID != kept.ID {
		t.Errorf("thread = %+v, want only the reply worth keeping", replies)
	}

	// And the whole report takes the rest of the conversation with it.
	if err := store.Delete(ctx, record.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	left, err := store.Replies(ctx, nil, record.ID)
	if err != nil {
		t.Fatalf("replies: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("%d replies outlived the report they were about", len(left))
	}
}

func TestAReplyNeedsSomethingInIt(t *testing.T) {
	store, author, _ := fixture(t)
	ctx := context.Background()

	record, err := store.Create(ctx, author.ID, report(KindBug, PriorityLow, "Something"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for name, body := range map[string]string{
		"blank": "   ",
		"long":  strings.Repeat("x", MaxBodyChars+1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := store.AddReply(ctx, ReplyInput{
				FeedbackID: record.ID, UserID: author.ID, Body: body, RequireOwner: true,
			})
			if !errors.Is(err, ErrInvalidBody) {
				t.Errorf("err = %v, want ErrInvalidBody", err)
			}
		})
	}
	if _, err := store.AddReply(ctx, ReplyInput{
		FeedbackID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", UserID: author.ID, Body: "x",
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("reply to a thread that is not there = %v, want ErrNotFound", err)
	}
}
