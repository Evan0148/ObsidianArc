package web

import (
	"bytes"
	"html"
)

// replaceTitle swaps the shell's <title> text for the configured browser
// title. A byte-level substitution rather than html/template: the document is
// this project's own build output, not anything untrusted, and pulling in a
// templating package to fill in one tag would be the kind of dependency
// AGENTS.md asks to stop and question rather than reach for.
//
// Absent or malformed markup is left untouched: a title an operator cannot
// see is a smaller failure than a handler that panics because a future build
// changed how the shell writes its own tag.
func replaceTitle(index []byte, title string) []byte {
	const open = "<title>"
	const close = "</title>"

	start := bytes.Index(index, []byte(open))
	if start < 0 {
		return index
	}
	contentStart := start + len(open)
	end := bytes.Index(index[contentStart:], []byte(close))
	if end < 0 {
		return index
	}
	end += contentStart

	// Escaped because, unlike the colour below, this is free text an operator
	// typed into a settings field — it ends up inside markup, not inside an
	// attribute value with its own quoting.
	return spliced(index, contentStart, end, []byte(html.EscapeString(title)))
}

// replaceThemeColor swaps the shell's static theme-color value for the
// operator's configured one. The tag always exists — a fresh instance ships
// the same neutral default the interface's own theme picker starts from — so
// this only ever finds a value to replace.
func replaceThemeColor(index []byte, color string) []byte {
	const marker = `<meta name="theme-color" content="`

	start := bytes.Index(index, []byte(marker))
	if start < 0 {
		return index
	}
	valueStart := start + len(marker)
	valueEnd := bytes.IndexByte(index[valueStart:], '"')
	if valueEnd < 0 {
		return index
	}
	valueEnd += valueStart

	// Not escaped: callers only ever pass a value settings.ValidHexColor has
	// already accepted, which is `#` and six hex digits — none of them
	// meaningful to an HTML attribute parser.
	return spliced(index, valueStart, valueEnd, []byte(color))
}

// spliced returns index with the bytes between start and end replaced by
// replacement. Shared by both substitutions above so there is one place that
// gets the byte arithmetic right rather than two.
func spliced(index []byte, start, end int, replacement []byte) []byte {
	out := make([]byte, 0, len(index)-(end-start)+len(replacement))
	out = append(out, index[:start]...)
	out = append(out, replacement...)
	out = append(out, index[end:]...)
	return out
}
