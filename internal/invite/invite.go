// Package invite owns invite codes: an administrator's batch, an account's
// own personal code, and the one atomic operation both are spent through.
//
// Two shapes of code share one table rather than two, because the thing that
// has to be exactly right — consuming a code exactly once under concurrent
// registrations — is one statement, and a second table would be a second
// copy of it to keep honest. What differs between an admin batch and a
// personal code is only which columns are set: owner_id, a group and its
// trial length for the former; nothing but an unlimited max_uses for the
// latter.
package invite

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	mathrand "math/rand/v2"
	"regexp"
	"strings"
	"time"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/card"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/database"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/id"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/notify"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/settings"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// Status is computed from a code's own columns at read time rather than
// stored: it is a pure function of revoked_at, expires_at, max_uses and uses,
// and a stored copy would just be a second value that could disagree with
// the columns that actually govern what Consume does.
const (
	StatusActive  = "active"
	StatusUsedUp  = "used_up"
	StatusExpired = "expired"
	StatusRevoked = "revoked"
)

func status(revokedAt, expiresAt int64, maxUses, uses int, now int64) string {
	switch {
	case revokedAt != 0:
		return StatusRevoked
	case expiresAt != 0 && expiresAt <= now:
		return StatusExpired
	case maxUses != 0 && uses >= maxUses:
		return StatusUsedUp
	default:
		return StatusActive
	}
}

// Code is one row, as an administrator or an owning account sees it.
type Code struct {
	ID            string `json:"id"`
	Code          string `json:"code"`
	OwnerID       string `json:"owner_id"`
	OwnerUsername string `json:"owner_username"`
	OwnerNickname string `json:"owner_nickname"`
	GroupID       string `json:"group_id"`
	GroupName     string `json:"group_name"`
	GroupDays     int    `json:"group_days"`
	GroupDaysMax  int    `json:"group_days_max"`
	MaxUses       int    `json:"max_uses"`
	Uses          int    `json:"uses"`
	ExpiresAt     int64  `json:"expires_at"`
	RevokedAt     int64  `json:"revoked_at"`
	Note          string `json:"note"`
	CreatedBy     string `json:"created_by"`
	CreatedAt     int64  `json:"created_at"`
	Status        string `json:"status"`
}

// Use is one registration through a code, as an owner or an administrator
// reads it back. Reward is spelled as two fields rather than an enum: a
// reader needs "did this count" (rewarded) and, when it did not, why
// (reward_skipped) — collapsing those into one value would make the zero
// case ("not resolved yet", still possible right after registration when
// verification is required) indistinguishable from "resolved, not rewarded".
type Use struct {
	UserID        string `json:"user_id"`
	Username      string `json:"username"`
	Nickname      string `json:"nickname"`
	GroupDays     int64  `json:"group_days"`
	CreatedAt     int64  `json:"created_at"`
	RewardedAt    int64  `json:"rewarded_at"`
	RewardCards   int    `json:"reward_cards"`
	RewardSkipped string `json:"reward_skipped"`
}

var (
	ErrNotFound         = errors.New("invite: no such code")
	ErrInvalid          = errors.New("invite: that code is not valid")
	ErrCodeTaken        = errors.New("invite: that code already exists")
	ErrCodeFormat       = errors.New("invite: a custom code must be 4-32 characters of A-Z and 0-9")
	ErrNamedBatch       = errors.New("invite: a batch is generated, so it cannot be given a code of its own")
	ErrInvalidCount     = errors.New("invite: count must be between 1 and 500")
	ErrInvalidMaxUses   = errors.New("invite: max uses must be between 0 and 100000")
	ErrInvalidGroupDays = errors.New("invite: group days must be between 0 and 3650, and group_days_max must be 0 or at least group_days")
	ErrGroupRequired    = errors.New("invite: group_days_max requires a group")
)

const (
	MaxBatch     = 500
	MaxNoteChars = 200
	MaxGroupDays = 3650
	MaxMaxUses   = 100000
	// A generated code's own fixed length — see generatedAlphabet. A custom
	// one's 4-32 character bound lives only in customCodeRE below and in
	// ErrCodeFormat's message: nothing else in this package cares what the
	// bound actually is.
	generatedLen = 8
)

// generatedAlphabet excludes 0, 1, I, L, O and U — every character that can
// be misread for another on a screen or mistyped from a photo of one. Thirty
// symbols wide, so eight of them is close enough to the entropy a UUID's
// first segment carries that guessing one is not a realistic attack; it does
// not have to be a secret on the order of a password, only unguessable
// enough that grep-ing sequential codes is not a strategy.
const generatedAlphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"

var customCodeRE = regexp.MustCompile(`^[A-Z0-9]{4,32}$`)

// Normalise is the one folding rule for a code, applied to what an
// administrator types, what a registration form sends and what is stored:
// upper-cased, with spaces and hyphens removed. "abcd-1234", "ABCD 1234" and
// "abcd1234" all name the same row because they all normalise to the same
// string.
func Normalise(raw string) string {
	var out strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(raw)) {
		if r == ' ' || r == '-' {
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// ValidCustom reports whether a normalised string is an acceptable
// administrator-named code: 4-32 characters of A-Z and 0-9. Deliberately
// wider than the generated alphabet — a partner's own chosen word is not
// read off a screen the way a generated one is, so there is no reason to
// keep it off 0, 1, I, L, O or U.
func ValidCustom(normalised string) bool { return customCodeRE.MatchString(normalised) }

// Display puts the hyphen back for an eight-character code — the shape every
// generated code is — and returns anything else, a custom one, exactly as
// stored: an administrator who chose "PARTNERX" typed it without a hyphen
// and should see it without one.
func Display(code string) string {
	if len(code) == generatedLen {
		return code[:4] + "-" + code[4:]
	}
	return code
}

// generateCode returns something a person can read off a screen and type
// without asking which character that was — see generatedAlphabet.
func generateCode() (string, error) {
	picked, err := drawSymbols(rand.Reader, generatedLen, len(generatedAlphabet))
	if err != nil {
		return "", fmt.Errorf("invite: generate code: %w", err)
	}
	var out strings.Builder
	for _, symbol := range picked {
		out.WriteByte(generatedAlphabet[symbol])
	}
	return out.String(), nil
}

// drawSymbols returns count uniform indices into an alphabet width symbols
// wide. Rejection sampling rather than `int(b) % width`, the same reasoning
// card.drawSymbols carries: 256 is not a multiple of 30, so the naive
// spelling would favour the alphabet's first characters by a small but real
// margin in a value whose whole job is to be unguessable.
func drawSymbols(reader io.Reader, count, width int) ([]byte, error) {
	if width < 1 || width > 256 {
		return nil, fmt.Errorf("invite: alphabet width %d", width)
	}
	ceiling := 256 - 256%width

	picked := make([]byte, 0, count)
	batch := make([]byte, count)
	for len(picked) < count {
		if _, err := io.ReadFull(reader, batch); err != nil {
			return nil, err
		}
		for _, b := range batch {
			if int(b) >= ceiling {
				continue
			}
			picked = append(picked, byte(int(b)%width))
			if len(picked) == count {
				break
			}
		}
	}
	return picked, nil
}

// Store owns invite_codes and invite_uses.
type Store struct {
	db       *database.DB
	users    *user.Store
	cards    *card.Store
	settings *settings.Service
	// Where a reward tells its inviter it landed. Set by the wiring, the
	// same seam every other store that raises a notice uses; nil means
	// "push nothing", which is every instance predating this feature and
	// every test with no reason to exercise it.
	Notify *notify.Store
}

func NewStore(db *database.DB, users *user.Store, cards *card.Store, set *settings.Service) *Store {
	return &Store{db: db, users: users, cards: cards, settings: set}
}

// Settings reads back the invite settings a caller building a response needs
// without reaching past this package into internal/settings' key names
// itself — the profile handler is the one call site, and this keeps the
// key constants from having to be exported knowledge outside this package
// and internal/admin.
func (s *Store) Settings() (userEnabled bool, limit, rewardCards, rewardCardDays int) {
	return s.settings.Bool(settings.InvitesUserEnabled),
		s.settings.Int(settings.InvitesUserLimit, 10),
		s.settings.Int(settings.InvitesRewardCards, 0),
		s.settings.Int(settings.InvitesRewardCardDays, 30)
}

// Grant is what Consume hands back: which code was spent, who gets credited
// for the invite (empty for an admin-issued code), and the group membership
// it carries. auth.InviteGrant mirrors this shape — see server.go's wiring
// — rather than this package being imported by internal/auth: auth is
// imported back by this package's own HTTP handlers (auth.RequireUser), and
// a two-way import between them does not compile.
type Grant struct {
	CodeID    string
	Code      string
	OwnerID   string
	GroupID   string
	GroupDays int64
}

// Consume spends one use of a code atomically, inside the caller's
// transaction — the registration that is asking must commit or roll back
// together with the use it spends, so a registration that fails for any
// other reason (a taken username, a throttle) hands the use back for free.
//
// Every failure collapses to ErrInvalid on purpose: an unknown code, a
// revoked one, an expired one, one already at its limit and one whose owner
// has been disabled all read the same to whoever typed it. Telling those
// apart would tell an attacker which guesses are close.
func (s *Store) Consume(ctx context.Context, q database.Queryer, rawCode string, now int64) (*Grant, error) {
	code := Normalise(rawCode)
	if code == "" {
		return nil, ErrInvalid
	}

	var (
		codeID, ownerID, groupID string
		groupDays, groupDaysMax  int
	)
	err := q.QueryRow(ctx,
		`SELECT id, owner_id, group_id, group_days, group_days_max FROM invite_codes WHERE code = ?`, code).
		Scan(&codeID, &ownerID, &groupID, &groupDays, &groupDaysMax)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrInvalid
		}
		return nil, fmt.Errorf("invite: read code: %w", err)
	}

	// A personal code's owner can be disabled or removed after being
	// handed out. One code for all in the error either way, for the same
	// reason every other rejection here is: this is checked before the
	// spend below rather than after, so a disabled owner's code never
	// takes a use it is about to refuse anyway.
	if ownerID != "" {
		// Personal codes stop working when the operator switches personal
		// invites off, not only stop being handed out.
		userEnabled, limit, _, _ := s.Settings()
		if !userEnabled {
			return nil, ErrInvalid
		}
		// The owner's row is held until the registration commits: the
		// owner's status and their count of invites are read here and
		// relied on by the spend below, and an operator disabling the owner
		// or a second registration through the same code must wait.
		locked, err := q.Exec(ctx, `UPDATE users SET updated_at = updated_at WHERE id = ?`, ownerID)
		if err != nil {
			return nil, fmt.Errorf("invite: lock owner: %w", err)
		}
		if present, err := locked.RowsAffected(); err != nil {
			return nil, fmt.Errorf("invite: lock owner: %w", err)
		} else if present != 1 {
			return nil, ErrInvalid
		}
		var ownerStatus string
		if err := q.QueryRow(ctx, `SELECT status FROM users WHERE id = ?`, ownerID).Scan(&ownerStatus); err != nil {
			return nil, fmt.Errorf("invite: read owner: %w", err)
		}
		if ownerStatus != string(user.StatusActive) {
			return nil, ErrInvalid
		}
		// The per-account limit is on invites, not only on rewards: a
		// personal code carries an unlimited max_uses of its own, and this
		// is what stops it at the operator's number.
		if limit > 0 {
			var invited int
			if err := q.QueryRow(ctx, `SELECT COUNT(*) FROM invite_uses WHERE inviter_id = ?`, ownerID).
				Scan(&invited); err != nil {
				return nil, fmt.Errorf("invite: count invites: %w", err)
			}
			if invited >= limit {
				return nil, ErrInvalid
			}
		}
	}

	// The one atomic step: every condition Consume promises is re-checked
	// here, in the WHERE clause, so nothing between the SELECT above and
	// this UPDATE can be raced. Two registrations spending the last use of
	// a max_uses=1 code concurrently both read the row above; only one's
	// UPDATE matches a row.
	result, err := q.Exec(ctx,
		`UPDATE invite_codes SET uses = uses + 1
		 WHERE id = ? AND revoked_at = 0
		   AND (max_uses = 0 OR uses < max_uses)
		   AND (expires_at = 0 OR expires_at > ?)`,
		codeID, now)
	if err != nil {
		return nil, fmt.Errorf("invite: consume: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("invite: consume: %w", err)
	}
	if affected != 1 {
		return nil, ErrInvalid
	}

	if groupID != "" {
		// A group named on the code can be deleted after being issued. The
		// same rule auth.Service.registrationGroup applies to the instance
		// default: a dangling reference is dropped rather than failing a
		// registration that has already spent its use.
		var exists int
		switch err := q.QueryRow(ctx, `SELECT 1 FROM user_groups WHERE id = ?`, groupID).Scan(&exists); {
		case database.IsNotFound(err):
			groupID, groupDays, groupDaysMax = "", 0, 0
		case err != nil:
			return nil, fmt.Errorf("invite: read group: %w", err)
		}
	}

	days := int64(groupDays)
	if groupID != "" && groupDaysMax > groupDays {
		// math/rand/v2, not crypto/rand: a trial length is not a secret,
		// and this only has to spread registrations across a range, not
		// resist being guessed.
		days = int64(groupDays + mathrand.IntN(groupDaysMax-groupDays+1))
	}

	return &Grant{CodeID: codeID, Code: code, OwnerID: ownerID, GroupID: groupID, GroupDays: days}, nil
}

// RecordUse writes the row Consume's spend is remembered by, inside the same
// transaction. Kept separate from Consume rather than folded into it because
// the caller (Register or Provision) decides the user id only after Consume
// has already resolved the group — the account does not exist yet when
// Consume runs.
func (s *Store) RecordUse(ctx context.Context, q database.Queryer, codeID, userID, inviterID string, groupDays int64) error {
	_, err := q.Exec(ctx,
		`INSERT INTO invite_uses (user_id, code_id, inviter_id, group_days, created_at, rewarded_at, reward_cards, reward_skipped)
		 VALUES (?, ?, ?, ?, ?, 0, 0, '')`,
		userID, codeID, inviterID, groupDays, time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("invite: record use: %w", err)
	}
	return nil
}

// Reward resolves the personal-code invite reward for userID: whether the
// invitee now qualifies to trigger it, and if so whether the inviter is
// actually paid or the reward is skipped and why.
//
// Called right after registration, again when the account verifies its
// email, and by RewardPending on the janitor's pass — "qualifies" can change
// between them: an account created unverified qualifies for nothing on an
// instance that requires verification, and one the signup review restricted
// qualifies once the restriction lifts, which nothing tells this package
// about. verificationRequired is passed in rather than
// read from a setting here because whether verification is actually in
// force also depends on whether mail is configured at all — knowledge that
// belongs to auth.Service.VerificationRequired, not duplicated here.
func (s *Store) Reward(ctx context.Context, userID string, verificationRequired bool) error {
	var (
		codeID, inviterID string
		rewardedAt        int64
	)
	err := s.db.QueryRow(ctx,
		`SELECT code_id, inviter_id, rewarded_at FROM invite_uses WHERE user_id = ?`, userID).
		Scan(&codeID, &inviterID, &rewardedAt)
	if err != nil {
		if database.IsNotFound(err) {
			// Never registered through a code at all.
			return nil
		}
		return fmt.Errorf("invite: read use: %w", err)
	}
	// Admin-issued codes have nobody to reward, and a use already resolved
	// (granted or skipped) is not resolved twice.
	if inviterID == "" || rewardedAt != 0 {
		return nil
	}

	invitee, err := s.users.ByID(ctx, nil, userID)
	if err != nil {
		return fmt.Errorf("invite: read invitee: %w", err)
	}
	now := time.Now()
	qualifies := invitee.IsActive() && !invitee.APIRestrictedAt(now) &&
		(!verificationRequired || invitee.EmailVerified)
	if !qualifies {
		// Left unclaimed on purpose: the next call — at verification, if
		// that is what this account is still waiting on — asks again.
		return nil
	}

	// One transaction, holding the inviter's row, for the whole decision.
	// The limit is read and then written, so two invitees qualifying at
	// once must not both find the inviter under it; and the claim commits
	// only together with the cards it pays, so a grant that fails leaves
	// the use unresolved for the next attempt rather than marked paid.
	var (
		reason  string
		cards   int
		claimed bool
	)
	err = s.db.Tx(ctx, func(tx *database.Tx) error {
		locked, err := tx.Exec(ctx, `UPDATE users SET updated_at = updated_at WHERE id = ?`, inviterID)
		if err != nil {
			return fmt.Errorf("invite: lock inviter: %w", err)
		}
		present, err := locked.RowsAffected()
		if err != nil {
			return fmt.Errorf("invite: lock inviter: %w", err)
		}
		var days int
		reason, cards, days, err = s.evaluateReward(ctx, tx, invitee, inviterID, present == 1)
		if err != nil {
			return err
		}

		// The claim: a second caller for the same invitee — registration
		// and an eager click on the verification link, say — affects no
		// row once this one has committed, and returns having done nothing.
		result, err := tx.Exec(ctx,
			`UPDATE invite_uses SET rewarded_at = ?, reward_cards = ?, reward_skipped = ?
			 WHERE user_id = ? AND rewarded_at = 0`,
			now.UnixMilli(), cards, reason, userID)
		if err != nil {
			return fmt.Errorf("invite: claim reward: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("invite: claim reward: %w", err)
		}
		if affected != 1 {
			return nil
		}
		claimed = true
		if reason != "" {
			return nil
		}
		if _, err := s.cards.Grant(ctx, tx, inviterID, cards, days); err != nil {
			return fmt.Errorf("invite: grant reward cards: %w", err)
		}
		return nil
	})
	if err != nil || !claimed || reason != "" {
		return err
	}

	if s.Notify != nil {
		if err := s.Notify.Push(ctx, nil, notify.Notification{
			Audience: notify.AudienceUser, UserID: inviterID,
			Kind:   "invite_joined",
			Params: map[string]any{"username": invitee.Username, "cards": cards},
			Link:   "/settings?tab=invites",
		}); err != nil {
			return fmt.Errorf("invite: notify inviter: %w", err)
		}
	}
	return nil
}

// RewardPending asks again for every recent use still waiting on a reward.
// Registration and verification are the two moments Reward is otherwise
// called, and an invitee can come to qualify at neither: a restriction the
// AI review placed at sign-up that later runs out, or that an operator
// lifts. Bounded to the last month and a page at a time, so a backlog is
// worked through over a few passes instead of all at once.
func (s *Store) RewardPending(ctx context.Context, verificationRequired bool) error {
	rows, err := s.db.Query(ctx,
		`SELECT user_id FROM invite_uses
		 WHERE rewarded_at = 0 AND inviter_id <> '' AND created_at > ?
		 ORDER BY created_at LIMIT 200`,
		time.Now().Add(-30*24*time.Hour).UnixMilli())
	if err != nil {
		return fmt.Errorf("invite: read pending rewards: %w", err)
	}
	var pending []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			rows.Close()
			return fmt.Errorf("invite: read pending rewards: %w", err)
		}
		pending = append(pending, userID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("invite: read pending rewards: %w", err)
	}
	for _, userID := range pending {
		if err := s.Reward(ctx, userID, verificationRequired); err != nil {
			return err
		}
	}
	return nil
}

// evaluateReward decides whether the inviter is actually paid, and how much,
// once the invitee is known to qualify at all. q is Reward's transaction,
// which already holds the inviter's row: every read here goes through it,
// since on SQLite a second connection would wait behind that very lock.
func (s *Store) evaluateReward(
	ctx context.Context, q database.Queryer, invitee user.User, inviterID string, present bool,
) (reason string, cards, days int, err error) {
	userEnabled, limit, rewardCards, rewardCardDays := s.Settings()

	// An inviter deleted since the invite has nobody to pay, and one
	// disabled since has been judged by an operator; either way the use is
	// resolved rather than left for the cards to fail against forever.
	if !present {
		return "inviter_gone", 0, 0, nil
	}
	inviter, err := s.users.ByID(ctx, q, inviterID)
	if err != nil {
		return "", 0, 0, fmt.Errorf("invite: read inviter: %w", err)
	}
	if !inviter.IsActive() {
		return "inviter_disabled", 0, 0, nil
	}
	// Switching personal invites off stops the payouts too, not only new
	// codes: the operator turning it off is usually doing so because of how
	// it was being used.
	if !userEnabled || rewardCards <= 0 {
		return "disabled", 0, 0, nil
	}
	if sameIP(ctx, q, invitee, inviter) {
		return "same_ip", 0, 0, nil
	}
	if limit > 0 {
		already, err := rewardedCount(ctx, q, inviterID)
		if err != nil {
			return "", 0, 0, err
		}
		if already >= limit {
			return "limit", 0, 0, nil
		}
	}
	return "", rewardCards, rewardCardDays, nil
}

// sameIP catches an inviter rewarding themselves: a second account signed up
// from the address the first registered from, or from an address the first
// is still signed in on.
func sameIP(ctx context.Context, q database.Queryer, invitee, inviter user.User) bool {
	if invitee.SignupIP == "" {
		return false
	}
	if inviter.SignupIP != "" && inviter.SignupIP == invitee.SignupIP {
		return true
	}
	var exists int
	// Best effort: a read failure here must not be able to leave a reward
	// stuck forever, so it reads as "no match" rather than failing Reward.
	err := q.QueryRow(ctx,
		`SELECT 1 FROM sessions WHERE user_id = ? AND ip = ? AND expires_at > ? LIMIT 1`,
		inviter.ID, invitee.SignupIP, time.Now().UnixMilli()).Scan(&exists)
	return err == nil
}

func rewardedCount(ctx context.Context, q database.Queryer, inviterID string) (int, error) {
	var count int
	err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM invite_uses WHERE inviter_id = ? AND rewarded_at <> 0 AND reward_skipped = ''`,
		inviterID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("invite: count rewards: %w", err)
	}
	return count, nil
}

// PersonalCode returns account's own invite code, creating one under its row
// lock if it does not already have a live one — so two tabs opening the
// invites panel at once cannot mint two.
func (s *Store) PersonalCode(ctx context.Context, userID string) (Code, error) {
	var record Code
	err := s.db.Tx(ctx, func(tx *database.Tx) error {
		// Per-account invariant: lock the owner's row, the AGENTS.md
		// spelling.
		if _, err := tx.Exec(ctx, `UPDATE users SET updated_at = updated_at WHERE id = ?`, userID); err != nil {
			return fmt.Errorf("invite: lock account: %w", err)
		}
		existing, err := s.ownedCode(ctx, tx, userID)
		if err == nil {
			record = existing
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		record, err = s.createPersonal(ctx, tx, userID)
		return err
	})
	if err != nil {
		return Code{}, err
	}
	return record, nil
}

// Regenerate revokes the current personal code and issues a new one, under
// the same row lock PersonalCode uses.
func (s *Store) Regenerate(ctx context.Context, userID string) (Code, error) {
	var record Code
	err := s.db.Tx(ctx, func(tx *database.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET updated_at = updated_at WHERE id = ?`, userID); err != nil {
			return fmt.Errorf("invite: lock account: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE invite_codes SET revoked_at = ? WHERE owner_id = ? AND revoked_at = 0`,
			time.Now().UnixMilli(), userID); err != nil {
			return fmt.Errorf("invite: revoke personal code: %w", err)
		}
		var err error
		record, err = s.createPersonal(ctx, tx, userID)
		return err
	})
	if err != nil {
		return Code{}, err
	}
	return record, nil
}

func (s *Store) ownedCode(ctx context.Context, q database.Queryer, userID string) (Code, error) {
	now := time.Now().UnixMilli()
	var record Code
	err := q.QueryRow(ctx,
		`SELECT id, code, max_uses, uses, expires_at, revoked_at, created_at
		 FROM invite_codes WHERE owner_id = ? AND revoked_at = 0
		 ORDER BY created_at DESC LIMIT 1`, userID).
		Scan(&record.ID, &record.Code, &record.MaxUses, &record.Uses,
			&record.ExpiresAt, &record.RevokedAt, &record.CreatedAt)
	if err != nil {
		if database.IsNotFound(err) {
			return Code{}, ErrNotFound
		}
		return Code{}, fmt.Errorf("invite: read personal code: %w", err)
	}
	record.OwnerID = userID
	record.Status = status(record.RevokedAt, record.ExpiresAt, record.MaxUses, record.Uses, now)
	return record, nil
}

// createPersonal mints a fresh, unlimited-use code for an account. A retry
// budget rather than one attempt: a collision on the generated text is
// vanishingly rare with thirty symbols and eight characters, but costs
// nothing to retry and should never reach a caller as a failure.
func (s *Store) createPersonal(ctx context.Context, q database.Queryer, userID string) (Code, error) {
	now := time.Now().UnixMilli()
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		generated, err := generateCode()
		if err != nil {
			return Code{}, err
		}
		record := Code{ID: id.New(), Code: generated, OwnerID: userID, CreatedAt: now, Status: StatusActive}
		_, err = q.Exec(ctx, `INSERT INTO invite_codes
			(id, code, owner_id, group_id, group_days, group_days_max, max_uses, uses, expires_at, revoked_at, note, created_by, created_at)
			VALUES (?, ?, ?, '', 0, 0, 0, 0, 0, 0, '', ?, ?)`,
			record.ID, record.Code, record.OwnerID, userID, record.CreatedAt)
		if err == nil {
			return record, nil
		}
		if !isUnique(err) {
			return Code{}, fmt.Errorf("invite: create personal code: %w", err)
		}
		lastErr = err
	}
	return Code{}, fmt.Errorf("invite: create personal code: %w", lastErr)
}

func isUnique(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate") ||
		strings.Contains(message, "constraint")
}

// Uses lists every registration through one code, newest first — an
// account's own list when the code is its personal one, or an
// administrator's view of one it issued.
func (s *Store) Uses(ctx context.Context, codeID string) ([]Use, error) {
	rows, err := s.db.Query(ctx, `
		SELECT iu.user_id, COALESCE(u.username, ''), COALESCE(u.nickname, ''),
		       iu.group_days, iu.created_at, iu.rewarded_at, iu.reward_cards, iu.reward_skipped
		FROM invite_uses iu
		JOIN users u ON u.id = iu.user_id
		WHERE iu.code_id = ?
		ORDER BY iu.created_at DESC`, codeID)
	if err != nil {
		return nil, fmt.Errorf("invite: list uses: %w", err)
	}
	defer rows.Close()

	out := []Use{}
	for rows.Next() {
		var record Use
		if err := rows.Scan(&record.UserID, &record.Username, &record.Nickname,
			&record.GroupDays, &record.CreatedAt, &record.RewardedAt,
			&record.RewardCards, &record.RewardSkipped); err != nil {
			return nil, fmt.Errorf("invite: scan use: %w", err)
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("invite: list uses: %w", err)
	}
	return out, nil
}
