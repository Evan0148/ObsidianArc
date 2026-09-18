package consolessh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// loadOrCreateHostKey returns the console's SSH identity, generating and
// persisting one on first start. Idempotent: a second call against the same
// path reads back exactly what the first one wrote, which is what lets
// `ssh admin@host` show the same fingerprint across restarts instead of
// tripping every client's known-hosts check on every deploy.
//
// The key is ed25519 for the same reason internal/auth prefers Argon2id over
// rolling its own: one well-reviewed primitive, no size/curve choice to get
// wrong, and it is the format golang.org/x/crypto/ssh round-trips natively
// through MarshalPrivateKey/ParsePrivateKey with no extra encoding step.
func loadOrCreateHostKey(path string) (ssh.Signer, error) {
	if path == "" {
		return nil, errors.New("consolessh: host key path is empty")
	}

	if existing, err := os.ReadFile(path); err == nil {
		signer, parseErr := ssh.ParsePrivateKey(existing)
		if parseErr != nil {
			return nil, fmt.Errorf("consolessh: parse host key %s: %w", path, parseErr)
		}
		return signer, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("consolessh: read host key %s: %w", path, err)
	}

	// Mirrors internal/config's secret-file pattern: 0700 directory, 0600
	// file. A host key under a custom OBSIDIAN_SSH_HOST_KEY path should not
	// depend on its parent directory already existing.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("consolessh: create host key directory: %w", err)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("consolessh: generate host key: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "obsidian-arc console host key")
	if err != nil {
		return nil, fmt.Errorf("consolessh: marshal host key: %w", err)
	}
	encoded := pem.EncodeToMemory(block)
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return nil, fmt.Errorf("consolessh: write host key %s: %w", path, err)
	}

	// Round-tripping through ParsePrivateKey rather than building the
	// signer straight from priv means a corrupt write is caught here, at
	// startup, instead of surfacing as a handshake failure to the first
	// person who tries to connect.
	signer, err := ssh.ParsePrivateKey(encoded)
	if err != nil {
		return nil, fmt.Errorf("consolessh: parse newly written host key: %w", err)
	}
	return signer, nil
}

// HostKeyFingerprint loads or creates the host key at path and returns its
// fingerprint without standing a server up.
//
// It exists for one ordering problem in the wiring: `help ssh` prints the
// fingerprint, so the console engine needs it — but the SSH server needs the
// console engine, and the two cannot both be built second. Loading the key is
// idempotent, so the wiring reads the fingerprint here first and the server
// reads the same key back a moment later.
func HostKeyFingerprint(path string) (string, error) {
	signer, err := loadOrCreateHostKey(path)
	if err != nil {
		return "", err
	}
	return fingerprint(signer), nil
}

// fingerprint renders a host key the same way OpenSSH's own `ssh-keygen -lf`
// does, which is what an administrator compares against `help ssh`'s output
// before trusting a new console.
func fingerprint(signer ssh.Signer) string {
	return ssh.FingerprintSHA256(signer.PublicKey())
}
