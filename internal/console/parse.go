package console

import (
	"fmt"
	"strings"
	"unicode"
)

// Tokenize splits a line into words, the way a shell would for the subset
// the console needs: single quotes are literal (no escapes inside them),
// double quotes allow \" and \\ to escape themselves, and a backslash
// outside any quote escapes the very next character. Any other run of
// unicode whitespace separates words — not just the space bar, since the
// web terminal's Shift+Enter can paste a multi-line `setting set` in one
// submission.
func Tokenize(line string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	hasToken := false

	runes := []rune(line)
	i := 0
	for i < len(runes) {
		ch := runes[i]
		switch {
		case unicode.IsSpace(ch):
			if hasToken {
				tokens = append(tokens, cur.String())
				cur.Reset()
				hasToken = false
			}
			i++

		case ch == '\'':
			hasToken = true
			i++
			start := i
			for i < len(runes) && runes[i] != '\'' {
				i++
			}
			if i >= len(runes) {
				return nil, fmt.Errorf("console: unterminated ' quote")
			}
			cur.WriteString(string(runes[start:i]))
			i++ // skip the closing '

		case ch == '"':
			hasToken = true
			i++
			for i < len(runes) && runes[i] != '"' {
				if runes[i] == '\\' && i+1 < len(runes) && (runes[i+1] == '"' || runes[i+1] == '\\') {
					cur.WriteRune(runes[i+1])
					i += 2
					continue
				}
				cur.WriteRune(runes[i])
				i++
			}
			if i >= len(runes) {
				return nil, fmt.Errorf(`console: unterminated " quote`)
			}
			i++ // skip the closing "

		case ch == '\\' && i+1 < len(runes):
			hasToken = true
			cur.WriteRune(runes[i+1])
			i += 2

		default:
			hasToken = true
			cur.WriteRune(ch)
			i++
		}
	}
	if hasToken {
		tokens = append(tokens, cur.String())
	}
	return tokens, nil
}

// ParsedArgs is what ParseFlags separated a token stream into.
type ParsedArgs struct {
	Args []string
	// Flags maps a flag's canonical long name, without leading dashes, to
	// its raw value — "true" for a boolean flag given bare. -h/--help,
	// --json and -y/--yes are never in here; they are pulled out into
	// their own fields because every command answers to them, declared or
	// not (see registry.go's Flag doc).
	Flags map[string]string
	Help  bool
	JSON  bool
	Yes   bool
}

// ParseFlags walks tokens against a command's declared flags: "--flag
// value" and "--flag=value" for a value-taking flag (Value != ""), a bare
// "--flag" or "-x" for a boolean one, "--" to stop flag parsing entirely
// (everything after it is positional, however it looks), and an error for
// anything starting with "-" that matches no declared flag and is not one
// of the three universal ones above.
func ParseFlags(tokens []string, flags []Flag) (ParsedArgs, error) {
	parsed := ParsedArgs{Flags: map[string]string{}}

	byLong := make(map[string]Flag, len(flags))
	byShort := make(map[string]Flag, len(flags))
	for _, f := range flags {
		byLong[normalizeFlagName(f.Name)] = f
		if f.Short != "" {
			byShort[normalizeFlagName(f.Short)] = f
		}
	}

	positionalOnly := false
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]

		if positionalOnly {
			parsed.Args = append(parsed.Args, tok)
			continue
		}

		switch {
		case tok == "--":
			positionalOnly = true

		case tok == "-h" || tok == "--help":
			parsed.Help = true

		case tok == "--json":
			parsed.JSON = true

		case tok == "-y" || tok == "--yes":
			parsed.Yes = true

		case strings.HasPrefix(tok, "--") && tok != "--":
			body := tok[2:]
			name, value, hasEq := strings.Cut(body, "=")
			f, ok := byLong[name]
			if !ok {
				return ParsedArgs{}, fmt.Errorf("console: unknown flag --%s", name)
			}
			if f.Value == "" { // boolean
				if hasEq {
					parsed.Flags[name] = value
				} else {
					parsed.Flags[name] = "true"
				}
				continue
			}
			if hasEq {
				parsed.Flags[name] = value
				continue
			}
			if i+1 >= len(tokens) {
				return ParsedArgs{}, fmt.Errorf("console: flag --%s requires a value", name)
			}
			i++
			parsed.Flags[name] = tokens[i]

		case strings.HasPrefix(tok, "-") && tok != "-":
			short := tok[1:]
			f, ok := byShort[short]
			if !ok {
				return ParsedArgs{}, fmt.Errorf("console: unknown flag %s", tok)
			}
			long := normalizeFlagName(f.Name)
			if f.Value == "" {
				parsed.Flags[long] = "true"
				continue
			}
			if i+1 >= len(tokens) {
				return ParsedArgs{}, fmt.Errorf("console: flag %s requires a value", tok)
			}
			i++
			parsed.Flags[long] = tokens[i]

		default:
			parsed.Args = append(parsed.Args, tok)
		}
	}

	return parsed, nil
}
