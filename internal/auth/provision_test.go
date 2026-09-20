package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/database"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/settings"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// provision runs one Provision in its own transaction, the way a provider
// callback does.
func provision(t *testing.T, f *fixture, in ProvisionInput) (user.User, error) {
	t.Helper()
	var created user.User
	err := f.db.Tx(context.Background(), func(tx *database.Tx) error {
		account, err := f.auth.Provision(context.Background(), tx, in)
		created = account
		return err
	})
	return created, err
}

func TestProvisionOpensAnAccountWithNoPassword(t *testing.T) {
	f := newFixture(t)

	account, err := provision(t, f, ProvisionInput{Username: "octocat", Email: "cat@example.com"})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if account.Username != "octocat" {
		t.Errorf("username = %q, want the provider's", account.Username)
	}
	// The first account on an empty instance, however it arrives.
	if account.Role != user.RoleSuperAdmin {
		t.Errorf("role = %q, want the first account to administer", account.Role)
	}
	if account.GroupID == "" {
		t.Error("the account was not put in a group")
	}
	// An address a provider proved is confirmed; nothing is posted to it.
	if !account.EmailVerified {
		t.Error("a proven address arrived unconfirmed")
	}

	hash, err := f.users.PasswordHash(context.Background(), nil, account.ID)
	if err != nil {
		t.Fatalf("read credential: %v", err)
	}
	if hash != "" {
		t.Errorf("password hash = %q, want none at all", hash)
	}
}

// The one thing a passwordless account must not do is answer the sign-in
// form. A stored hash that cannot be parsed used to reach the caller as an
// internal error, which is both a 500 where a 401 belongs and a way to ask
// the form which accounts have a password.
func TestAnAccountWithNoPasswordRefusesTheSignInFormLikeAnyOther(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, _, err := f.auth.Register(ctx, RegisterInput{
		Username: "founder", Password: "a-good-password",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	account, err := provision(t, f, ProvisionInput{Username: "octocat"})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}

	for _, attempt := range []string{"", "a-good-password", "anything at all"} {
		_, _, err := f.auth.Login(ctx, LoginInput{Identifier: account.Username, Password: attempt})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("login with %q = %v, want the same refusal a wrong password gets", attempt, err)
		}
	}
	if _, err := f.auth.VerifyCredential(ctx, account.Username, "a-good-password", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("console credential check = %v, want the same refusal", err)
	}
}

// And the way out of that state: the owner sets a first password without
// being asked for one they have never had. Without this, an operator who
// switched a provider off would have locked those accounts out for good.
func TestAPasswordlessAccountCanSetItsFirstPassword(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	account, err := provision(t, f, ProvisionInput{Username: "octocat"})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if err := f.auth.ChangePassword(ctx, account.ID, "", "a-good-password", ""); err != nil {
		t.Fatalf("set first password: %v", err)
	}
	if _, _, err := f.auth.Login(ctx, LoginInput{
		Identifier: "octocat", Password: "a-good-password",
	}); err != nil {
		t.Fatalf("sign in with the new password: %v", err)
	}
	// And from then on it is an ordinary password: the current one is
	// required again.
	if err := f.auth.ChangePassword(ctx, account.ID, "", "another-password", ""); !errors.Is(err, ErrCurrentPasswordWrong) {
		t.Errorf("second change without the current password = %v, want a refusal", err)
	}
}

func TestProvisionFindsAFreeSpellingOfATakenName(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, _, err := f.auth.Register(ctx, RegisterInput{
		Username: "octocat", Password: "a-good-password",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	account, err := provision(t, f, ProvisionInput{Username: "octocat"})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if account.Username == "octocat" {
		t.Fatal("the provider's name was taken and was used anyway")
	}
	if !strings.HasPrefix(account.Username, "octocat") {
		t.Errorf("username = %q, want it derived from the provider's", account.Username)
	}
	if err := user.ValidateUsername(account.Username); err != nil {
		t.Errorf("username %q is not storable: %v", account.Username, err)
	}
}

// Nobody types the name here, so a name this column cannot hold is not an
// error to report — there is nobody to report it to. It becomes one that can.
func TestProvisionMakesAUsableNameOutOfAnUnusableOne(t *testing.T) {
	f := newFixture(t)

	for _, suggestion := range []string{"", "黑曜", "..", "a", strings.Repeat("x", 80), "  spaced name  "} {
		account, err := provision(t, f, ProvisionInput{Username: suggestion})
		if err != nil {
			t.Fatalf("provision %q: %v", suggestion, err)
		}
		if err := user.ValidateUsername(account.Username); err != nil {
			t.Errorf("%q became %q, which is not storable: %v", suggestion, account.Username, err)
		}
	}
}

// Every registration control the sign-up form obeys, obeyed here too. The
// whole reason this lives in auth rather than in the provider package is that
// this list cannot be allowed to exist twice.
func TestProvisionObeysTheRegistrationControls(t *testing.T) {
	ctx := context.Background()

	t.Run("closed registration", func(t *testing.T) {
		f := newFixture(t)
		if _, _, err := f.auth.Register(ctx, RegisterInput{
			Username: "founder", Password: "a-good-password",
		}); err != nil {
			t.Fatalf("register: %v", err)
		}
		if err := f.settings.Set(ctx, settings.RegistrationEnabled, "false"); err != nil {
			t.Fatalf("close registration: %v", err)
		}
		if _, err := provision(t, f, ProvisionInput{Username: "octocat"}); !errors.Is(err, ErrRegistrationClosed) {
			t.Errorf("provision = %v, want the closed-registration refusal", err)
		}
	})

	t.Run("domain allowlist", func(t *testing.T) {
		f := newFixture(t)
		if _, _, err := f.auth.Register(ctx, RegisterInput{
			Username: "founder", Password: "a-good-password",
		}); err != nil {
			t.Fatalf("register: %v", err)
		}
		if err := f.settings.Set(ctx, settings.EmailDomains, "company.com"); err != nil {
			t.Fatalf("set domains: %v", err)
		}
		_, err := provision(t, f, ProvisionInput{Username: "octocat", Email: "cat@gmail.com"})
		var domain *EmailDomainError
		if !errors.As(err, &domain) {
			t.Errorf("provision = %v, want the address refused for its domain", err)
		}
		if _, err := provision(t, f, ProvisionInput{
			Username: "insider", Email: "someone@company.com",
		}); err != nil {
			t.Errorf("an allowed domain was refused: %v", err)
		}
	})

	t.Run("a required QQ number cannot be supplied", func(t *testing.T) {
		f := newFixture(t)
		if _, _, err := f.auth.Register(ctx, RegisterInput{
			Username: "founder", Password: "a-good-password", QQ: "12345678",
		}); err != nil {
			t.Fatalf("register: %v", err)
		}
		if err := f.settings.Set(ctx, settings.QQRequirement, settings.QQRequired); err != nil {
			t.Fatalf("require qq: %v", err)
		}
		if _, err := provision(t, f, ProvisionInput{Username: "octocat"}); !errors.Is(err, user.ErrQQRequired) {
			t.Errorf("provision = %v, want it refused rather than opening an account that breaks the rule", err)
		}
	})

	t.Run("per-address limit", func(t *testing.T) {
		f := newFixture(t)
		if _, _, err := f.auth.Register(ctx, RegisterInput{
			Username: "founder", Password: "a-good-password",
		}); err != nil {
			t.Fatalf("register: %v", err)
		}
		if err := f.settings.Set(ctx, settings.SignupsPerIP, "2"); err != nil {
			t.Fatalf("set the limit: %v", err)
		}
		for i := 0; i < 2; i++ {
			if _, err := provision(t, f, ProvisionInput{
				Username: fmt.Sprintf("visitor%d", i), IP: "203.0.113.9",
			}); err != nil {
				t.Fatalf("provision %d: %v", i, err)
			}
		}
		if _, err := provision(t, f, ProvisionInput{
			Username: "visitor3", IP: "203.0.113.9",
		}); !errors.Is(err, ErrSignupIPBlocked) {
			t.Errorf("third from one address = %v, want it blocked", err)
		}
		// Another address is unaffected: the limit is per address, not a
		// gate on the whole instance.
		if _, err := provision(t, f, ProvisionInput{
			Username: "elsewhere", IP: "198.51.100.4",
		}); err != nil {
			t.Errorf("another address was caught by the per-address limit: %v", err)
		}
	})
}

// Two callbacks for two different people arriving at once, on an empty
// instance. Real goroutines, because the failure this guards against — both
// counting zero accounts and both becoming the administrator — only happens
// when the two reads interleave.
func TestParallelProvisionsMakeOneAdministrator(t *testing.T) {
	f := newFixture(t)
	start := make(chan struct{})
	var workers sync.WaitGroup

	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			<-start
			_, _ = provision(t, f, ProvisionInput{Username: fmt.Sprintf("visitor%d", index)})
		}(i)
	}
	close(start)
	workers.Wait()

	admins, err := f.users.CountActiveAdmins(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("count administrators: %v", err)
	}
	if admins != 1 {
		t.Errorf("administrators = %d, want exactly one", admins)
	}
}
