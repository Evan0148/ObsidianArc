package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/database"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/settings"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// Accounts opened by something other than the sign-up form.
//
// Today that is a provider sign-in: GitHub or Google has already established
// who is at the browser, and what is left is the part this instance owns —
// whether it is accepting accounts at all, which addresses it accepts, how
// fast they may appear, which group they land in, and whether this is the
// first account and therefore the administrator.
//
// That list lives here rather than in internal/oauth on purpose. It is the
// same list Register applies, and two copies of it would drift the first time
// an operator asked for a rule the sign-up form has and the provider button
// does not — which is exactly the shape of bug the sign-up controls exist to
// prevent.

// ErrNoUsernameAvailable means every candidate derived from the provider's
// name was taken. Vanishingly unlikely, and better than a loop that never
// ends.
var ErrNoUsernameAvailable = errors.New("auth: no username could be derived for this account")

// ProvisionInput is an account described by a provider rather than by a form.
type ProvisionInput struct {
	// A suggestion. A taken one is suffixed rather than refused: nobody
	// pressing "continue with GitHub" chose a username here, so there is
	// nobody to tell that theirs is unavailable.
	Username string
	// Empty unless the provider states the address is verified. An address
	// this instance has not seen proof of is not written to an account, which
	// is why nothing below asks for a confirmation link: there is either a
	// proven address or none at all.
	Email    string
	Nickname string
	IP       string
	UA       string
}

// Provision creates the account.
//
// It runs inside the caller's transaction and takes the instance lock itself,
// so a provider sign-in and a form registration cannot both decide they are
// the first account, and the per-address count cannot be read by two of them
// before either has written.
//
// Deliberately not run past the sign-up reviewer. That review reads a
// username, an address and a user agent and judges whether a person chose
// them; here nobody chose them, and the provider has already done the work of
// establishing that an account exists somewhere with a history. Sending it a
// row it cannot judge would produce a verdict about nothing.
func (s *Service) Provision(ctx context.Context, tx *database.Tx, in ProvisionInput) (user.User, error) {
	if err := settings.Lock(ctx, tx); err != nil {
		return user.User{}, err
	}

	total, err := s.users.Count(ctx, tx)
	if err != nil {
		return user.User{}, err
	}
	first := total == 0

	// None of the registration controls apply to the first account, for the
	// reason Register gives: it is the one that turns an empty instance into
	// an administered one.
	if !first {
		if !s.settings.Bool(settings.RegistrationEnabled) {
			return user.User{}, ErrRegistrationClosed
		}
		if err := checkEmail(s.settings, in.Email); err != nil {
			return user.User{}, err
		}
		// A provider has no QQ number to offer and never will. An instance
		// that requires one is asking for something this route cannot supply,
		// so it says so rather than opening an account that breaks the rule.
		if s.settings.Get(settings.QQRequirement) == settings.QQRequired {
			return user.User{}, user.ErrQQRequired
		}
		if allowed, retryAfter := s.signups.allow(
			s.settings.Int(settings.SignupsPerMinute, 0),
			s.settings.Int(settings.SignupsPerHour, 0),
		); !allowed {
			return user.User{}, &SignupThrottleError{RetryAfter: retryAfter}
		}
		if err := s.checkSignupIP(ctx, tx, in.IP); err != nil {
			return user.User{}, err
		}
	}

	username, err := s.availableUsername(ctx, tx, in.Username)
	if err != nil {
		return user.User{}, err
	}
	// The address is the one thing here somebody else may already hold. The
	// caller looks first and links instead where it does, so reaching this
	// with a taken address means the two accounts are not the same person —
	// the provider's proof is for an address this instance gave away.
	if strings.TrimSpace(in.Email) != "" {
		_, emailTaken, _, err := s.users.Exists(ctx, tx, username, in.Email, "")
		if err != nil {
			return user.User{}, err
		}
		if emailTaken {
			return user.User{}, user.ErrEmailTaken
		}
	}

	groupID, err := s.registrationGroup(ctx, tx)
	if err != nil {
		return user.User{}, err
	}
	role := user.RoleUser
	if first {
		role = user.RoleSuperAdmin
	}

	created, err := s.users.Create(ctx, tx, user.CreateInput{
		Username: username,
		Email:    in.Email,
		Nickname: in.Nickname,
		// No password. Not a placeholder and not a random one nobody knows: a
		// credential that exists is a credential that can be guessed at, and
		// this account has never had one. Login answers an attempt against it
		// exactly as it answers a wrong password; the owner can set one from
		// their own settings, which is the only place that knows it is them.
		PasswordHash: "",
		Role:         role,
		GroupID:      groupID,
		Status:       user.StatusActive,
		// Nothing to confirm: the address arrived proven or not at all.
		Unverified:      false,
		SignupIP:        in.IP,
		SignupUserAgent: in.UA,
	})
	if err != nil {
		return user.User{}, err
	}

	// Inside the lock, for the reason Register gives: recording after commit
	// leaves a gap in which the next queued sign-up passes a throttle this
	// one should already have moved.
	s.signups.record()
	return created, nil
}

// StartSession issues a session for an account something else has already
// authenticated.
//
// It exists so that session policy — the lifetime, the address and client
// recorded against it, the login timestamp — stays in one place instead of
// being reimplemented by every caller that can establish who somebody is.
func (s *Service) StartSession(ctx context.Context, account user.User, ip, ua string) (string, error) {
	token, _, err := s.sessions.Create(ctx, account.ID, s.cfg.TTL, ip, ua)
	if err != nil {
		return "", err
	}
	_ = s.users.MarkLogin(ctx, account.ID, time.Now().UnixMilli())
	return token, nil
}

// CheckEmail applies the instance's address rules to an address that did not
// come from the sign-up form. Exported for the same reason ParseDomains is:
// the rule is the operator's, and it has to hold on every way in.
func CheckEmail(set *settings.Service, email string) error { return checkEmail(set, email) }

// availableUsername turns a provider's name for somebody into one this
// instance can store, then finds a spelling nobody has taken.
func (s *Service) availableUsername(ctx context.Context, q database.Queryer, suggestion string) (string, error) {
	base := sanitiseUsername(suggestion)

	candidates := make([]string, 0, 15)
	candidates = append(candidates, base)
	for n := 2; n <= 9; n++ {
		candidates = append(candidates, truncateUsername(base, 31)+strconv.Itoa(n))
	}
	// Then four random digits. Counting further would walk an attacker
	// straight to "who already has this name"; a random tail just works.
	for attempt := 0; attempt < 5; attempt++ {
		candidates = append(candidates, truncateUsername(base, 28)+randomDigits(4))
	}

	for _, candidate := range candidates {
		if err := user.ValidateUsername(candidate); err != nil {
			continue
		}
		taken, _, _, err := s.users.Exists(ctx, q, candidate, "", "")
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	return "", ErrNoUsernameAvailable
}

// sanitiseUsername keeps what user.ValidateUsername accepts and drops the
// rest. A name written in a script this column does not hold — which is most
// of them — comes out empty and gets the generic base, because a username
// nobody chose only has to be unique and readable.
func sanitiseUsername(raw string) string {
	var out strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			out.WriteRune(r)
		}
	}
	// Leading and trailing punctuation is accepted by the pattern and reads
	// as a mistake on a profile.
	cleaned := strings.Trim(out.String(), "._-")
	if len(cleaned) < 3 {
		return "user"
	}
	return truncateUsername(cleaned, 32)
}

func truncateUsername(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return strings.Trim(value[:limit], "._-")
}

func randomDigits(count int) string {
	var out strings.Builder
	for i := 0; i < count; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			// Same reasoning as the state nonce: there is no sensible
			// fallback for missing randomness, and carrying on with a
			// predictable one is worse than stopping.
			panic(fmt.Sprintf("auth: no randomness available: %v", err))
		}
		out.WriteString(n.String())
	}
	return out.String()
}
