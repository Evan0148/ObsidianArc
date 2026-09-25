package totp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// RFC 6238, appendix B, the SHA-1 column. The RFC prints eight digits; the
// same computation with a six-digit modulus is what an app shows.
func TestRFC6238Vectors(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).
		EncodeToString([]byte("12345678901234567890"))
	for _, tc := range []struct {
		at   int64
		want string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	} {
		step := Step(time.Unix(tc.at, 0))
		got, err := code(secret, step, 8)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("T=%d: got %s, want %s", tc.at, got, tc.want)
		}
		short, _ := Code(secret, step)
		if short != tc.want[2:] {
			t.Errorf("T=%d six digits: got %s, want %s", tc.at, short, tc.want[2:])
		}
	}
}

func TestMatchAcceptsOneStepEitherSideAndNoMore(t *testing.T) {
	secret := NewSecret()
	now := time.Unix(1_700_000_000, 0)
	for delta := int64(-3); delta <= 3; delta++ {
		code, _ := Code(secret, Step(now)+delta)
		step, ok, err := Match(secret, code, now)
		if err != nil {
			t.Fatal(err)
		}
		want := delta >= -Skew && delta <= Skew
		if ok != want {
			t.Errorf("delta %d: matched=%v, want %v", delta, ok, want)
		}
		if ok && step != Step(now)+delta {
			t.Errorf("delta %d: reported step %d, want %d", delta, step, Step(now)+delta)
		}
	}
}

// The space an app draws in the middle of a code is what people copy.
func TestMatchIgnoresSeparators(t *testing.T) {
	secret := NewSecret()
	now := time.Now()
	code, _ := Code(secret, Step(now))
	if _, ok, _ := Match(secret, code[:3]+" "+code[3:], now); !ok {
		t.Fatal("a code with a space in the middle did not match")
	}
	if !LooksLikeCode(code[:3] + " " + code[3:]) {
		t.Fatal("a spaced code was not recognised as a code")
	}
}

func TestSecretIsTypeableBase32(t *testing.T) {
	secret := NewSecret()
	if len(secret) != 32 || strings.ContainsAny(secret, "=018") {
		t.Fatalf("secret %q is not 32 characters of unpadded base32", secret)
	}
	if NewSecret() == secret {
		t.Fatal("two secrets were the same")
	}
}

func TestURICarriesIssuerAndSecret(t *testing.T) {
	uri := URI("Obsidian Arc", "arc", "JBSWY3DPEHPK3PXP")
	want := "otpauth://totp/Obsidian%20Arc:arc?secret=JBSWY3DPEHPK3PXP&issuer=Obsidian%20Arc&algorithm=SHA1&digits=6&period=30"
	if uri != want {
		t.Fatalf("got  %s\nwant %s", uri, want)
	}
	// An issuer is the operator's text, and a stray '&' in it must not start
	// a parameter of its own.
	if uri := URI("A&B", "arc", "X"); !strings.Contains(uri, "issuer=A%26B&") {
		t.Fatalf("an ampersand in the issuer was not escaped: %s", uri)
	}
}

func TestRecoveryCodesNormalise(t *testing.T) {
	codes := RecoveryCodes()
	if len(codes) != RecoveryCount {
		t.Fatalf("got %d codes", len(codes))
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if len(code) != 11 || code[5] != '-' {
			t.Fatalf("code %q is not xxxxx-xxxxx", code)
		}
		normal := NormalizeRecovery(code)
		if normal == "" || seen[normal] {
			t.Fatalf("code %q normalised to %q", code, normal)
		}
		seen[normal] = true
		if NormalizeRecovery(strings.ToUpper(code)) != normal {
			t.Fatalf("upper case %q did not normalise the same", code)
		}
		if LooksLikeCode(code) {
			t.Fatalf("recovery code %q was taken for a one-time code", code)
		}
	}
	// Read off paper: an I for a 1 and an O for a 0 are the same code.
	if NormalizeRecovery("IO234-56789") != "10234"+"56789" {
		t.Fatal("ambiguous letters were not read as digits")
	}
	if NormalizeRecovery("short") != "" || NormalizeRecovery("uuuuu-uuuuu") != "" {
		t.Fatal("something that cannot be a code normalised to one")
	}
}
