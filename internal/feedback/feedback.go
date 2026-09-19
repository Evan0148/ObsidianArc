// Package feedback is what the people using the instance have to say about
// it: a defect they hit, or something they would like it to do.
//
// Rows rather than a mail relay, for the same reason announcements are rows:
// "what have people told us, and what did we do about it" is worth being
// able to look up afterwards, and a report that arrives in one operator's
// mailbox is a report the next operator never sees.
//
// The body is plain text held as written. It is shown only in the
// backoffice, to an administrator, inside a Vue template that escapes what
// it interpolates — so nothing here renders markup, and nothing here needs
// to sanitise any.
package feedback

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/database"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/id"
)

// Kind is what the reader is telling us: something is broken, or something
// could be better. Two, because a third ("question") would be a support
// channel, and this is not one.
type Kind string

const (
	KindBug  Kind = "bug"
	KindIdea Kind = "idea"
)

var Kinds = []Kind{KindBug, KindIdea}

func (k Kind) Valid() bool {
	for _, candidate := range Kinds {
		if k == candidate {
			return true
		}
	}
	return false
}

// Priority is the reporter's own estimate of how much it matters. It is
// deliberately theirs rather than the operator's: what an operator thinks
// belongs in Status, and the gap between the two is the interesting part.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
)

var Priorities = []Priority{PriorityLow, PriorityMedium, PriorityHigh}

func (p Priority) Valid() bool {
	for _, candidate := range Priorities {
		if p == candidate {
			return true
		}
	}
	return false
}

// Status is the operator's half. Two values, not a workflow: a report has
// either been dealt with or it has not, and every extra state is a state
// somebody has to remember to move things out of.
type Status string

const (
	StatusOpen     Status = "open"
	StatusResolved Status = "resolved"
)

var Statuses = []Status{StatusOpen, StatusResolved}

func (s Status) Valid() bool {
	for _, candidate := range Statuses {
		if s == candidate {
			return true
		}
	}
	return false
}

const (
	MaxTitleChars = 120
	// Long enough for reproduction steps and a stack trace pasted in, short
	// enough that one report cannot be a denial-of-service on the table.
	MaxBodyChars = 8000
	// Per account, per rolling day. High enough that nobody reporting real
	// defects will meet it, low enough that a bored account cannot fill the
	// operator's list faster than it can be read.
	MaxPerDay = 10
)

// DayWindow is the span MaxPerDay is counted over.
const DayWindow = 24 * time.Hour

var (
	ErrNotFound      = errors.New("feedback: not found")
	ErrInvalidTitle  = errors.New("feedback: title must be 1-120 characters")
	ErrInvalidBody   = errors.New("feedback: body must be 1-8000 characters")
	ErrInvalidKind   = errors.New("feedback: unknown kind")
	ErrInvalidPrio   = errors.New("feedback: unknown priority")
	ErrInvalidStatus = errors.New("feedback: unknown status")
	ErrTooMany       = errors.New("feedback: this account has sent its daily maximum")
)

type Feedback struct {
	ID        string   `json:"id"`
	UserID    string   `json:"user_id"`
	Kind      Kind     `json:"kind"`
	Priority  Priority `json:"priority"`
	Status    Status   `json:"status"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	CreatedAt int64    `json:"created_at"`
	UpdatedAt int64    `json:"updated_at"`

	// Filled for the operator's listing, empty for the author's own — they
	// know who they are, and the join is not free.
	Username string `json:"username,omitempty"`
	Nickname string `json:"nickname,omitempty"`
}

type Store struct{ db *database.DB }

func NewStore(db *database.DB) *Store { return &Store{db: db} }

const columns = `f.id, f.user_id, f.kind, f.priority, f.status, f.title, f.body,
	f.created_at, f.updated_at`

func (s *Store) pick(q database.Queryer) database.Queryer {
	if q == nil {
		return s.db
	}
	return q
}

type Input struct {
	Kind     Kind
	Priority Priority
	Title    string
	Body     string
}

func validate(in Input) (Input, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > MaxTitleChars {
		return Input{}, ErrInvalidTitle
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" || utf8.RuneCountInString(in.Body) > MaxBodyChars {
		return Input{}, ErrInvalidBody
	}
	if in.Kind == "" {
		in.Kind = KindBug
	}
	if !in.Kind.Valid() {
		return Input{}, ErrInvalidKind
	}
	if in.Priority == "" {
		in.Priority = PriorityMedium
	}
	if !in.Priority.Valid() {
		return Input{}, ErrInvalidPrio
	}
	return in, nil
}

// Create records one report for userID.
//
// The daily cap and the insert share a transaction that locks the author's
// row first, for the reason apikey.Store.Issue and project.Store.Create do
// the same: a count followed by a standalone insert lets two parallel
// requests both see the last free slot and take it, and the deployment notes
// allow a second server process against one database, so a mutex here would
// only serialise half of them.
func (s *Store) Create(ctx context.Context, userID string, in Input) (Feedback, error) {
	in, err := validate(in)
	if err != nil {
		return Feedback{}, err
	}

	now := time.Now().UnixMilli()
	record := Feedback{
		ID:        id.New(),
		UserID:    userID,
		Kind:      in.Kind,
		Priority:  in.Priority,
		Status:    StatusOpen,
		Title:     in.Title,
		Body:      in.Body,
		CreatedAt: now,
		UpdatedAt: now,
	}

	err = s.db.Tx(ctx, func(tx *database.Tx) error {
		locked, err := tx.Exec(ctx,
			`UPDATE users SET updated_at = updated_at WHERE id = ?`, userID)
		if err != nil {
			return fmt.Errorf("feedback: lock author: %w", err)
		}
		if affected, rowsErr := locked.RowsAffected(); rowsErr == nil && affected == 0 {
			return ErrNotFound
		}

		var count int
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM feedback WHERE user_id = ? AND created_at > ?`,
			userID, now-DayWindow.Milliseconds()).Scan(&count); err != nil {
			return fmt.Errorf("feedback: count recent: %w", err)
		}
		if count >= MaxPerDay {
			return ErrTooMany
		}

		_, err = tx.Exec(ctx, `INSERT INTO feedback
			(id, user_id, kind, priority, status, title, body, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			record.ID, record.UserID, record.Kind, record.Priority, record.Status,
			record.Title, record.Body, record.CreatedAt, record.UpdatedAt)
		if err != nil {
			return fmt.Errorf("feedback: insert: %w", err)
		}
		return nil
	})
	if err != nil {
		return Feedback{}, err
	}
	return record, nil
}

// Filter is what the operator's list is narrowed by. Every field is
// optional, and an unknown enum value narrows to nothing rather than being
// ignored — a filter that silently matches everything is worse than one that
// visibly matches nothing.
type Filter struct {
	Kind     Kind
	Priority Priority
	Status   Status
	UserID   string
	Search   string
	Limit    int
	Offset   int
}

func (f Filter) where() (string, []any) {
	clauses := []string{}
	args := []any{}
	if f.Kind != "" {
		clauses = append(clauses, "f.kind = ?")
		args = append(args, string(f.Kind))
	}
	if f.Priority != "" {
		clauses = append(clauses, "f.priority = ?")
		args = append(args, string(f.Priority))
	}
	if f.Status != "" {
		clauses = append(clauses, "f.status = ?")
		args = append(args, string(f.Status))
	}
	if f.UserID != "" {
		clauses = append(clauses, "f.user_id = ?")
		args = append(args, f.UserID)
	}
	if search := strings.TrimSpace(f.Search); search != "" {
		// LOWER(...) LIKE rather than ILIKE: one of the two engines here has
		// never heard of ILIKE, and the migration lint says so.
		pattern := "%" + strings.ToLower(search) + "%"
		clauses = append(clauses, "(LOWER(f.title) LIKE ? OR LOWER(f.body) LIKE ? OR LOWER(u.username) LIKE ?)")
		args = append(args, pattern, pattern, pattern)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// List is the operator's view: every report, newest first, with the author
// resolved. total is the count before the page window, for the pager.
func (s *Store) List(ctx context.Context, q database.Queryer, filter Filter) (records []Feedback, total int, err error) {
	where, args := filter.where()
	queryer := s.pick(q)

	if err := queryer.QueryRow(ctx,
		`SELECT COUNT(*) FROM feedback f JOIN users u ON u.id = f.user_id`+where,
		args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("feedback: count: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := queryer.Query(ctx,
		`SELECT `+columns+`, u.username, u.nickname
		 FROM feedback f JOIN users u ON u.id = f.user_id`+where+
			` ORDER BY f.created_at DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("feedback: list: %w", err)
	}
	defer rows.Close()

	records = []Feedback{}
	for rows.Next() {
		var record Feedback
		if err := rows.Scan(&record.ID, &record.UserID, &record.Kind, &record.Priority,
			&record.Status, &record.Title, &record.Body, &record.CreatedAt, &record.UpdatedAt,
			&record.Username, &record.Nickname); err != nil {
			return nil, 0, fmt.Errorf("feedback: scan: %w", err)
		}
		records = append(records, record)
	}
	return records, total, rows.Err()
}

// ListFor is what an author sees: their own reports and their standing, so
// sending one twice because the first appeared to vanish is not a thing that
// happens.
func (s *Store) ListFor(ctx context.Context, q database.Queryer, userID string) ([]Feedback, error) {
	rows, err := s.pick(q).Query(ctx,
		`SELECT `+columns+` FROM feedback f WHERE f.user_id = ? ORDER BY f.created_at DESC LIMIT 50`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("feedback: list for author: %w", err)
	}
	defer rows.Close()

	out := []Feedback{}
	for rows.Next() {
		var record Feedback
		if err := rows.Scan(&record.ID, &record.UserID, &record.Kind, &record.Priority,
			&record.Status, &record.Title, &record.Body, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("feedback: scan: %w", err)
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *Store) ByID(ctx context.Context, q database.Queryer, feedbackID string) (Feedback, error) {
	var record Feedback
	err := s.pick(q).QueryRow(ctx,
		`SELECT `+columns+`, u.username, u.nickname
		 FROM feedback f JOIN users u ON u.id = f.user_id WHERE f.id = ?`, feedbackID).
		Scan(&record.ID, &record.UserID, &record.Kind, &record.Priority, &record.Status,
			&record.Title, &record.Body, &record.CreatedAt, &record.UpdatedAt,
			&record.Username, &record.Nickname)
	if err != nil {
		if database.IsNotFound(err) {
			return Feedback{}, ErrNotFound
		}
		return Feedback{}, fmt.Errorf("feedback: by id: %w", err)
	}
	return record, nil
}

// SetStatus is the whole of what an operator can change. The report itself
// is what somebody wrote, and it stays as they wrote it.
func (s *Store) SetStatus(ctx context.Context, feedbackID string, status Status) (Feedback, error) {
	if !status.Valid() {
		return Feedback{}, ErrInvalidStatus
	}
	result, err := s.db.Exec(ctx,
		`UPDATE feedback SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), time.Now().UnixMilli(), feedbackID)
	if err != nil {
		return Feedback{}, fmt.Errorf("feedback: set status: %w", err)
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr == nil && affected == 0 {
		return Feedback{}, ErrNotFound
	}
	return s.ByID(ctx, nil, feedbackID)
}

func (s *Store) Delete(ctx context.Context, feedbackID string) error {
	result, err := s.db.Exec(ctx, `DELETE FROM feedback WHERE id = ?`, feedbackID)
	if err != nil {
		return fmt.Errorf("feedback: delete: %w", err)
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr == nil && affected == 0 {
		return ErrNotFound
	}
	return nil
}

// Summary is the count block above the operator's list: how much there is,
// and how much of it is still waiting.
type Summary struct {
	Total    int `json:"total"`
	Open     int `json:"open"`
	Bugs     int `json:"bugs"`
	Ideas    int `json:"ideas"`
	HighOpen int `json:"high_open"`
}

// Counts answers the summary in one round trip. SUM(CASE …) rather than
// COUNT(…) FILTER: both engines here understand the first spelling, and the
// second is a newer standard than the oldest SQLite this is expected to run
// against. COALESCE because SUM over no rows is NULL on both of them, and a
// fresh instance has no rows.
func (s *Store) Counts(ctx context.Context, q database.Queryer) (Summary, error) {
	var out Summary
	err := s.pick(q).QueryRow(ctx, `SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN kind = ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN kind = ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = ? AND priority = ? THEN 1 ELSE 0 END), 0)
		FROM feedback`,
		string(StatusOpen), string(KindBug), string(KindIdea),
		string(StatusOpen), string(PriorityHigh)).
		Scan(&out.Total, &out.Open, &out.Bugs, &out.Ideas, &out.HighOpen)
	if err != nil {
		return Summary{}, fmt.Errorf("feedback: counts: %w", err)
	}
	return out, nil
}
