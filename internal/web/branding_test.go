package web

import (
	"strings"
	"testing"
)

const shellFixture = `<!doctype html>
<html><head>
<title>Obsidian Arc</title>
<link rel="icon" href="data:image/svg+xml,default">
<meta name="theme-color" content="#18181b">
<script>
  const stored = read();
  apply(stored);
</script>
</head></html>`

func TestReplaceTitle(t *testing.T) {
	out := string(replaceTitle([]byte(shellFixture), "My Instance"))
	if !strings.Contains(out, "<title>My Instance</title>") {
		t.Errorf("title was not replaced: %s", out)
	}

	// User-supplied text ends up inside markup, not inside an attribute, so
	// it has to be escaped or a title containing "</title>" would close the
	// tag early and leak the rest of the document into the visible text.
	out = string(replaceTitle([]byte(shellFixture), `<b>&"'`))
	if strings.Contains(out, "<title><b>") {
		t.Errorf("title was not escaped: %s", out)
	}
	if !strings.Contains(out, "&lt;b&gt;&amp;&#34;&#39;") {
		t.Errorf("expected escaped title text, got: %s", out)
	}
}

func TestReplaceTitleLeavesMalformedShellAlone(t *testing.T) {
	broken := []byte("<html><head></head></html>")
	if got := replaceTitle(broken, "My Instance"); string(got) != string(broken) {
		t.Errorf("a shell with no <title> should be returned unchanged, got: %s", got)
	}
}

func TestReplaceThemeColor(t *testing.T) {
	out := string(replaceThemeColor([]byte(shellFixture), "#ff8800"))
	if !strings.Contains(out, `<meta name="theme-color" content="#ff8800">`) {
		t.Errorf("theme-color was not replaced: %s", out)
	}
}

func TestReplaceThemeColorLeavesMalformedShellAlone(t *testing.T) {
	broken := []byte("<html><head></head></html>")
	if got := replaceThemeColor(broken, "#ff8800"); string(got) != string(broken) {
		t.Errorf("a shell with no theme-color meta should be returned unchanged, got: %s", got)
	}
}

func TestReplaceFavicon(t *testing.T) {
	out := string(replaceFavicon([]byte(shellFixture), "/api/site/logo?v=123"))
	if !strings.Contains(out, `<link rel="icon" href="/api/site/logo?v=123">`) {
		t.Errorf("favicon was not replaced: %s", out)
	}
}

func TestReplaceFaviconLeavesMalformedShellAlone(t *testing.T) {
	broken := []byte("<html><head></head></html>")
	if got := replaceFavicon(broken, "/logo.png"); string(got) != string(broken) {
		t.Errorf("a shell with no favicon link should be returned unchanged, got: %s", got)
	}
}

// The whole reason serveIndex is allowed to rewrite the title and the
// theme-color per request: the Content-Security-Policy hash in csp.go is
// computed once, from the same embedded index.html, by hashing the exact text
// inside every inline <script> that carries no src. Neither substitution here
// touches that text, so the hash a browser computes from the document it
// actually receives must still equal the one calculated at server startup —
// if this test ever fails, a later change made a substitution reach into the
// script body, and the shell's own theme-flash prevention would silently stop
// running for every operator who had ever set a browser title.
func TestBrandingSubstitutionsDoNotAffectInlineScriptHash(t *testing.T) {
	before := inlineScriptHashes(shellFixture)
	if len(before) != 1 {
		t.Fatalf("fixture has %d inline scripts, want 1", len(before))
	}

	branded := replaceFavicon(replaceThemeColor(replaceTitle([]byte(shellFixture), "A Very Different Title"), "#112233"), "/api/site/logo?v=1")
	after := inlineScriptHashes(string(branded))

	if len(after) != 1 || after[0] != before[0] {
		t.Errorf("branding changed the inline script hash: before %v, after %v", before, after)
	}
}
