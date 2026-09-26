package settings

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/database"
)

const (
	LoginBgLandscapeLight = "landscape_light"
	LoginBgLandscapeDark  = "landscape_dark"
	LoginBgPortraitLight  = "portrait_light"
	LoginBgPortraitDark   = "portrait_dark"
)

var ValidLoginBackgroundVariants = map[string]bool{
	LoginBgLandscapeLight: true,
	LoginBgLandscapeDark:  true,
	LoginBgPortraitLight:  true,
	LoginBgPortraitDark:   true,
}

func NormalizeVariant(raw string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(raw)), "-", "_")
}

const MaxLoginBackgroundBytes = 6 * 1024 * 1024

var (
	ErrLoginBackgroundUnsupported = errors.New("settings: login background must be JPEG, PNG, WebP or AVIF")
	ErrLoginBackgroundTooLarge    = errors.New("settings: login background is too large")
	ErrNoLoginBackground          = errors.New("settings: no login background set")
)

// DetectImageMedia verifies the file's magic signature bytes directly.
// Relying on client-supplied Content-Type headers alone is vulnerable to MIME
// spoofing, and standard library http.DetectContentType does not recognize AVIF.
func DetectImageMedia(data []byte) (string, error) {
	if len(data) < 12 {
		return "", ErrLoginBackgroundUnsupported
	}
	// PNG: \x89PNG\r\n\x1a\n
	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return "image/png", nil
	}
	// JPEG: \xFF\xD8\xFF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg", nil
	}
	// WebP: RIFF....WEBP
	if bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return "image/webp", nil
	}
	// AVIF: ....ftypavif or ....ftypavis or compatible brands in the ftyp box
	if bytes.Equal(data[4:8], []byte("ftyp")) {
		brand := string(data[8:12])
		if brand == "avif" || brand == "avis" {
			return "image/avif", nil
		}
		limit := min(len(data), 64)
		if bytes.Contains(data[8:limit], []byte("avif")) || bytes.Contains(data[8:limit], []byte("avis")) {
			return "image/avif", nil
		}
	}
	return "", ErrLoginBackgroundUnsupported
}

// LoginBackgrounds returns a copy of the current login background update timestamps.
func (s *Service) LoginBackgrounds() map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]int64, len(s.loginBackgrounds))
	for k, v := range s.loginBackgrounds {
		out[k] = v
	}
	return out
}

// SetLoginBackground stores or updates a login background image for a given variant.
func (s *Service) SetLoginBackground(ctx context.Context, variant, mime string, data []byte) (int64, error) {
	variant = NormalizeVariant(variant)
	if !ValidLoginBackgroundVariants[variant] {
		return 0, fmt.Errorf("settings: unknown variant %q", variant)
	}
	if len(data) == 0 || len(data) > MaxLoginBackgroundBytes {
		return 0, ErrLoginBackgroundTooLarge
	}

	detectedMime, err := DetectImageMedia(data)
	if err != nil {
		return 0, ErrLoginBackgroundUnsupported
	}
	mime = detectedMime

	at := time.Now().UnixMilli()
	_, err = s.db.Exec(ctx,
		`INSERT INTO login_backgrounds (variant, mime, data, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (variant) DO UPDATE SET
		   mime = excluded.mime,
		   data = excluded.data,
		   updated_at = excluded.updated_at`,
		variant, mime, data, at)
	if err != nil {
		return 0, fmt.Errorf("settings: save login background: %w", err)
	}

	s.mu.Lock()
	if s.loginBackgrounds == nil {
		s.loginBackgrounds = map[string]int64{}
	}
	s.loginBackgrounds[variant] = at
	s.mu.Unlock()
	return at, nil
}

// GetLoginBackground retrieves the mime, binary data, and update timestamp for a given variant.
func (s *Service) GetLoginBackground(ctx context.Context, variant string) (mime string, data []byte, at int64, err error) {
	variant = NormalizeVariant(variant)
	if !ValidLoginBackgroundVariants[variant] {
		return "", nil, 0, ErrNoLoginBackground
	}

	err = s.db.QueryRow(ctx,
		`SELECT mime, data, updated_at FROM login_backgrounds WHERE variant = ?`,
		variant).Scan(&mime, &data, &at)
	if err != nil {
		if database.IsNotFound(err) {
			return "", nil, 0, ErrNoLoginBackground
		}
		return "", nil, 0, fmt.Errorf("settings: read login background: %w", err)
	}
	return mime, data, at, nil
}

// DeleteLoginBackground removes a background image variant.
func (s *Service) DeleteLoginBackground(ctx context.Context, variant string) error {
	variant = NormalizeVariant(variant)
	if !ValidLoginBackgroundVariants[variant] {
		return nil
	}

	_, err := s.db.Exec(ctx, `DELETE FROM login_backgrounds WHERE variant = ?`, variant)
	if err != nil {
		return fmt.Errorf("settings: delete login background: %w", err)
	}

	s.mu.Lock()
	delete(s.loginBackgrounds, variant)
	s.mu.Unlock()
	return nil
}
