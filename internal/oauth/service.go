package oauth

import (
	"context"
	"errors"
	"strings"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/auth"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/database"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/settings"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// Turning "GitHub says this is user 4218" into an account of this instance's.
//
// Three outcomes, in this order: the identity is already connected and its
// account signs in; the provider has proved an address this instance already
// holds, and the two are the same person; or nobody here is that person yet
// and an account is opened. The order is the whole design — the subject is
// the identity, and the address is only ever a way to recognise an account
// that predates the connection.

var (
	// The address is spoken for and this sign-in may not adopt it: either the
	// operator turned that off, or the account already answers to a different
	// account at the same provider. Both have the same remedy, which is to
	// sign in the usual way and connect from the settings screen.
	ErrAddressTaken = errors.New("oauth: an account here already uses that address")
	// A provider identity nobody knows, on an instance that does not open
	// accounts this way.
	ErrSignupClosed = errors.New("oauth: this server does not open accounts from a provider sign-in")
	// Removing this connection would leave no way into the account.
	ErrLastWayIn = errors.New("oauth: this is the only way left into this account")
	// There was nothing to remove.
	ErrNotConnected = errors.New("oauth: that provider is not connected to this account")
)

// credentials names the three settings each provider is configured with.
//
// A map rather than a key built out of the provider id, so that the strings
// here are the same constants the settings screen and the writable list use,
// and TestEveryProviderHasItsSettings can tell when a provider is added
// without them.
var credentials = map[string][3]string{
	"github": {settings.OAuthGitHubEnabled, settings.OAuthGitHubID, settings.OAuthGitHubSecret},
	"google": {settings.OAuthGoogleEnabled, settings.OAuthGoogleID, settings.OAuthGoogleSecret},
}

type Service struct {
	db       *database.DB
	store    *Store
	users    *user.Store
	auth     *auth.Service
	settings *settings.Service
}

func NewService(
	db *database.DB, store *Store, users *user.Store, authService *auth.Service, set *settings.Service,
) *Service {
	return &Service{db: db, store: store, users: users, auth: authService, settings: set}
}

// Enabled reports whether the operator has both switched this provider on and
// finished configuring it. A button drawn for a provider with no client id is
// a button that leads to an apology.
func (s *Service) Enabled(providerID string) bool {
	keys, known := credentials[providerID]
	if !known {
		return false
	}
	return s.settings.Bool(keys[0]) && s.Credentials(providerID).configured()
}

func (s *Service) Credentials(providerID string) Credentials {
	keys, known := credentials[providerID]
	if !known {
		return Credentials{}
	}
	return Credentials{
		ClientID:     strings.TrimSpace(s.settings.Get(keys[1])),
		ClientSecret: strings.TrimSpace(s.settings.Get(keys[2])),
	}
}

// SignIn resolves a provider's answer to an account, opening one if this
// instance allows it.
func (s *Service) SignIn(ctx context.Context, identity Identity, ip, ua string) (user.User, error) {
	var account user.User

	err := s.db.Tx(ctx, func(tx *database.Tx) error {
		// The common path, and the cheap one: somebody signing in again.
		found, err := s.store.Account(ctx, tx, identity.Provider, identity.Subject)
		if err == nil {
			account, err = s.users.ByID(ctx, tx, found)
			if err != nil {
				return err
			}
			if !account.IsActive() {
				return auth.ErrAccountDisabled
			}
			return s.store.Touch(ctx, tx, identity)
		}
		if !errors.Is(err, ErrNoIdentity) {
			return err
		}

		// From here the transaction may create an account, so it takes the
		// instance lock — the one Register and Provision take — before
		// deciding anything. Provision takes it again and that is free; what
		// matters is that the lookup below and the write after it cannot be
		// split by another sign-in doing the same thing.
		if err := settings.Lock(ctx, tx); err != nil {
			return err
		}
		// Read again under the lock: the request that was ahead of this one
		// may have just connected this very identity.
		if found, err := s.store.Account(ctx, tx, identity.Provider, identity.Subject); err == nil {
			account, err = s.users.ByID(ctx, tx, found)
			if err != nil {
				return err
			}
			if !account.IsActive() {
				return auth.ErrAccountDisabled
			}
			return s.store.Touch(ctx, tx, identity)
		} else if !errors.Is(err, ErrNoIdentity) {
			return err
		}

		// An address the provider has proved, on an account that already
		// exists here: the same person, arriving a different way.
		if identity.Email != "" {
			existing, err := s.users.ByEmail(ctx, tx, identity.Email)
			switch {
			case err == nil:
				if !s.settings.Bool(settings.OAuthLinkByEmail) {
					return ErrAddressTaken
				}
				if !existing.IsActive() {
					return auth.ErrAccountDisabled
				}
				if err := s.store.Link(ctx, tx, existing.ID, identity); err != nil {
					// The account already answers to a different account at
					// this provider. Said as "taken" rather than as a
					// conflict, because from outside that is what it is.
					if errors.Is(err, ErrAlreadyLinked) {
						return ErrAddressTaken
					}
					return err
				}
				account = existing
				return nil
			case !errors.Is(err, user.ErrNotFound):
				return err
			}
		}

		// Nobody here is this person yet.
		if !s.settings.Bool(settings.OAuthAllowSignup) {
			// Unless there is nobody here at all. An empty instance is being
			// set up, and refusing the first account would leave a deployment
			// with no way in but the setting nobody can reach to change.
			populated, err := s.users.Any(ctx, tx)
			if err != nil {
				return err
			}
			if populated {
				return ErrSignupClosed
			}
		}

		created, err := s.auth.Provision(ctx, tx, auth.ProvisionInput{
			Username: identity.Login,
			Email:    identity.Email,
			Nickname: strings.TrimSpace(identity.Name),
			IP:       ip,
			UA:       ua,
		})
		if err != nil {
			return err
		}
		if err := s.store.Link(ctx, tx, created.ID, identity); err != nil {
			return err
		}
		account = created
		return nil
	})
	if err != nil {
		return user.User{}, err
	}
	return account, nil
}

// Connect adds a provider to an account that is already signed in.
func (s *Service) Connect(ctx context.Context, userID string, identity Identity) error {
	return s.db.Tx(ctx, func(tx *database.Tx) error {
		// The account's own row, because what follows is a check — is this
		// identity spoken for — and then a write against the same account.
		if _, err := tx.Exec(ctx,
			`UPDATE users SET updated_at = updated_at WHERE id = ?`, userID); err != nil {
			return err
		}
		switch existing, err := s.store.Account(ctx, tx, identity.Provider, identity.Subject); {
		case err == nil:
			if existing == userID {
				// Already connected, to this very account. Refreshing what
				// the provider says is the whole of the work.
				return s.store.Touch(ctx, tx, identity)
			}
			return ErrAlreadyLinked
		case !errors.Is(err, ErrNoIdentity):
			return err
		}
		return s.store.Link(ctx, tx, userID, identity)
	})
}

// Connections is what an account's settings screen shows, with the one fact
// the screen needs beside them: whether there is also a password, because
// that is what decides whether a connection may be removed.
func (s *Service) Connections(ctx context.Context, userID string) ([]Connection, bool, error) {
	items, err := s.store.For(ctx, nil, userID)
	if err != nil {
		return nil, false, err
	}
	hash, err := s.users.PasswordHash(ctx, nil, userID)
	if err != nil {
		return nil, false, err
	}
	return items, hash != "", nil
}

// Disconnect removes a connection, unless it is the last way in.
//
// An account with no password whose only connection is removed is an account
// nobody can reach — not disabled, not deleted, simply unreachable, with its
// conversations still in it. The check and the delete hold the account's row
// lock, because two clicks on two providers would otherwise each see the
// other still there and both go through.
func (s *Service) Disconnect(ctx context.Context, userID, provider string) error {
	return s.db.Tx(ctx, func(tx *database.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE users SET updated_at = updated_at WHERE id = ?`, userID); err != nil {
			return err
		}
		hash, err := s.users.PasswordHash(ctx, tx, userID)
		if err != nil {
			return err
		}
		count, err := s.store.Count(ctx, tx, userID)
		if err != nil {
			return err
		}
		if hash == "" && count <= 1 {
			return ErrLastWayIn
		}
		removed, err := s.store.Unlink(ctx, tx, userID, provider)
		if err != nil {
			return err
		}
		if !removed {
			return ErrNotConnected
		}
		return nil
	})
}
