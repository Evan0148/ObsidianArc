// Package totp is the arithmetic of an authenticator app: RFC 6238 one-time
// codes, the otpauth:// link that carries a secret into the app, and the
// recovery codes that stand in for the app when the phone is gone.
//
// SHA-1, six digits, thirty seconds. Those are not the strongest parameters
// the RFC allows; they are the only ones every authenticator app honours, and
// an app that silently ignores "SHA256" in the link shows a code that never
// matches. SHA-1's collision weakness is irrelevant inside an HMAC.
//
// Nothing here touches a database or knows about accounts. Whether a code has
// already been used, and whose secret it is, belong to internal/auth.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// Digits and Period are what the link advertises and what Code computes.
	Digits = 6
	Period = 30
	// 160 bits, the HMAC-SHA-1 block the RFC recommends and the length every
	// app accepts. It is 32 characters of base32, which is also a length a
	// person can type from the screen when the camera will not cooperate.
	SecretBytes = 20
	// One step either side of now. A phone whose clock is thirty seconds out,
	// or a person who typed the code just as it rolled over, still gets in;
	// anything wider hands a guesser more codes to hit.
	Skew = 1
)

// ErrSecret means the stored secret is not base32. It can only come from a
// corrupted row.
var ErrSecret = errors.New("totp: secret is not valid base32")

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a fresh secret in the form the link and the app expect.
func NewSecret() string {
	raw := make([]byte, SecretBytes)
	if _, err := rand.Read(raw); err != nil {
		// crypto/rand does not fail on a supported platform, and a secret
		// made from anything weaker would be worse than no second factor.
		panic("totp: crypto/rand unavailable: " + err.Error())
	}
	return encoding.EncodeToString(raw)
}

// Step is the counter a time falls in.
func Step(at time.Time) int64 { return at.Unix() / Period }

// Code is the code for one step.
func Code(secret string, step int64) (string, error) {
	return code(secret, step, Digits)
}

func code(secret string, step int64, digits int) (string, error) {
	key, err := encoding.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", ErrSecret
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)

	// RFC 4226's dynamic truncation: the low nibble of the last byte picks
	// where four bytes are read from, and the top bit is dropped so the value
	// is the same signed or unsigned.
	offset := sum[len(sum)-1] & 0x0F
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7FFFFFFF

	modulus := uint32(1)
	for i := 0; i < digits; i++ {
		modulus *= 10
	}
	out := strconv.FormatUint(uint64(value%modulus), 10)
	return strings.Repeat("0", digits-len(out)) + out, nil
}

// Match looks for the code in the window around now and returns the step it
// belongs to. The caller records that step, so the same code cannot be
// presented twice.
//
// Every candidate is compared, and in constant time, so how long this takes
// says nothing about which step — or whether any step — was close.
func Match(secret, candidate string, now time.Time) (int64, bool, error) {
	candidate = Normalize(candidate)
	current := Step(now)
	var (
		found int64
		ok    bool
	)
	for delta := int64(-Skew); delta <= Skew; delta++ {
		want, err := Code(secret, current+delta)
		if err != nil {
			return 0, false, err
		}
		if hmac.Equal([]byte(want), []byte(candidate)) && !ok {
			found, ok = current+delta, true
		}
	}
	return found, ok, nil
}

// Normalize drops what people type between digits: the space an app shows
// in the middle of a code, and whatever a paste brought along.
func Normalize(candidate string) string {
	var out strings.Builder
	for _, r := range candidate {
		if r >= '0' && r <= '9' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// LooksLikeCode says whether a string is shaped like a one-time code rather
// than a recovery code, so a single field can accept either.
func LooksLikeCode(candidate string) bool {
	trimmed := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' {
			return -1
		}
		return r
	}, strings.TrimSpace(candidate))
	return len(trimmed) == Digits && Normalize(trimmed) == trimmed
}

// URI is the otpauth:// link a QR code carries. The label is "issuer:account"
// and the issuer is repeated as a parameter, which is how both old and new
// apps learn which service the entry belongs to.
//
// Spaces are written as %20, never '+': several apps show a '+' in the name
// literally, and the issuer is the name somebody picks the right entry by.
func URI(issuer, account, secret string) string {
	escape := func(value string) string {
		return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
	}
	label := escape(account)
	if issuer != "" {
		label = escape(issuer) + ":" + label
	}
	query := "secret=" + secret
	if issuer != "" {
		query += "&issuer=" + escape(issuer)
	}
	query += "&algorithm=SHA1&digits=" + strconv.Itoa(Digits) + "&period=" + strconv.Itoa(Period)
	return "otpauth://totp/" + label + "?" + query
}

// --- recovery codes -----------------------------------------------------------

// RecoveryCount is how many codes an account is given at once. Ten is what
// people are used to, and enough that using one does not feel like spending
// the last.
const RecoveryCount = 10

// recoveryAlphabet is Crockford's base32 in lower case: no i, l, o or u, so a
// code read off paper cannot become a different valid code.
const recoveryAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// RecoveryCodes returns fresh codes, formatted the way they are shown:
// "xxxxx-xxxxx", fifty bits each.
func RecoveryCodes() []string {
	out := make([]string, RecoveryCount)
	raw := make([]byte, 10)
	for i := range out {
		if _, err := rand.Read(raw); err != nil {
			panic("totp: crypto/rand unavailable: " + err.Error())
		}
		var code strings.Builder
		for j, b := range raw {
			if j == 5 {
				code.WriteByte('-')
			}
			code.WriteByte(recoveryAlphabet[b&31])
		}
		out[i] = code.String()
	}
	return out
}

// NormalizeRecovery is the form a recovery code is compared in: lower case,
// no separators, and the letters Crockford reads as digits read as digits.
// Anything that cannot be a code comes back empty.
func NormalizeRecovery(candidate string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(candidate) {
		switch {
		case r == '-' || r == ' ':
			continue
		case r == 'i' || r == 'l':
			r = '1'
		case r == 'o':
			r = '0'
		}
		if !strings.ContainsRune(recoveryAlphabet, r) {
			return ""
		}
		out.WriteRune(r)
	}
	if out.Len() != 10 {
		return ""
	}
	return out.String()
}
