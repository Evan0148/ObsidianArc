package admin

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/settings"
)

// The settings screen sends every field it knows in one PUT, and
// updateSettings refuses the whole request on the first key that is not in
// writableSettings. So a field added to the form and forgotten here does not
// break that field — it breaks the entire screen, for every setting on it,
// with a message naming a key nobody typed.
//
// That is exactly what happened: `chat.agent_max_rounds` arrived with the
// work surface and was never added to the list, and from that commit until
// this test was written the settings page could not save anything at all.
// The two lists are in different languages in different directories, so
// nothing but a test that reads both could notice.
func TestEverySettingTheFormSendsIsWritable(t *testing.T) {
	source, err := os.ReadFile("../../web/src/views/admin/AdminSettings.vue")
	if err != nil {
		t.Fatalf("read the settings screen: %v", err)
	}

	// The payload is a literal object of 'dotted.key': value pairs. Matching
	// the quoted key is enough to find them all, and a key that appears only
	// in the load half is still a key this screen deals in.
	pattern := regexp.MustCompile(`'([a-z][a-z0-9_]*\.[a-z][a-z0-9_.]*)'`)
	matches := pattern.FindAllStringSubmatch(string(source), -1)
	if len(matches) == 0 {
		t.Fatal("found no setting keys in the form; the scanner has drifted from the source")
	}

	seen := map[string]bool{}
	for _, match := range matches {
		key := match[1]
		if seen[key] {
			continue
		}
		seen[key] = true
		if !writableSettings[key] {
			t.Errorf("the settings screen sends %q, which updateSettings refuses — "+
				"add it to writableSettings, or the whole screen stops saving", key)
		}
	}
}

// The other half of the same agreement: a key the server would accept but
// that is not a setting at all is a typo waiting to be found by an operator
// whose save does nothing.
func TestWritableSettingsAreRealKeys(t *testing.T) {
	for key := range writableSettings {
		if _, known := settings.Defaults[key]; known {
			continue
		}
		// The two Turnstile credentials have no default on purpose: an empty
		// one is what "not configured" means, and a default would be a key
		// that looks set.
		if strings.HasPrefix(key, "turnstile.") {
			continue
		}
		t.Errorf("writableSettings allows %q, which has no default and so is not a setting", key)
	}
}
