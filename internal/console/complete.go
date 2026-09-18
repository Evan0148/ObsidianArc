package console

import (
	"context"
	"strings"
)

// Completion is what one Tab press answers: replace line[From:pos] with
// any Item's Value to complete it.
type Completion struct {
	From  int
	Items []CompletionItem
}

// CompletionItem is one candidate. Value is what replaces the partial word;
// Label is what a client shows for it when that differs from Value (the
// full command name, for a noun-only match); Hint is the bilingual summary
// or flag description.
type CompletionItem struct {
	Value, Label, Hint string
}

// Complete answers a Tab press at byte offset pos in line. It completes
// either a command name (word by word: "user ed" completes to "user edit"
// one word at a time, exactly as "user " already typed in full lets it) or,
// once the word under the cursor starts with "-", a flag name — including
// the three universal ones (-h/--help, --json, and --yes/-y when the
// resolved command is destructive) that every command answers to whether
// or not it declares them.
//
// It only ever completes a command the actor may run: the same visibility
// rule help and Spec use.
func (c *Console) Complete(ctx context.Context, s *Session, line string, pos int) Completion {
	_ = ctx // no admin call is made for completion; kept for symmetry with Execute and for a future dynamic completer
	if pos < 0 {
		pos = 0
	}
	if pos > len(line) {
		pos = len(line)
	}
	prefix := line[:pos]

	from := 0
	for i := len(prefix) - 1; i >= 0; i-- {
		if prefix[i] == ' ' || prefix[i] == '\t' {
			from = i + 1
			break
		}
	}
	already := prefix[:from]
	partial := prefix[from:]

	if strings.HasPrefix(partial, "-") {
		return c.completeFlag(s, already, partial, from)
	}
	return c.completeName(s, already, partial, from)
}

func (c *Console) completeName(s *Session, already, partial string, from int) Completion {
	trimmed := strings.TrimRight(already, " \t")
	seen := map[string]bool{}
	var items []CompletionItem

	for _, cmd := range c.reg.visible(s.Actor) {
		var remainder string
		switch {
		case trimmed == "":
			remainder = cmd.Name
		case strings.HasPrefix(cmd.Name, trimmed+" "):
			remainder = strings.TrimPrefix(cmd.Name, trimmed+" ")
		default:
			continue
		}

		word := remainder
		if i := strings.IndexAny(remainder, " \t"); i >= 0 {
			word = remainder[:i]
		}
		if !strings.HasPrefix(word, partial) || seen[word] {
			continue
		}
		seen[word] = true

		label := cmd.Name
		if word != remainder {
			// Only the noun matched so far — the label names the group of
			// verbs under it, not one specific command.
			label = word
			if trimmed != "" {
				label = trimmed + " " + word
			}
		}
		items = append(items, CompletionItem{Value: word, Label: label, Hint: cmd.Summary.For(s.Lang)})
	}
	return Completion{From: from, Items: items}
}

func (c *Console) completeFlag(s *Session, already, partial string, from int) Completion {
	trimmed := strings.TrimRight(already, " \t")
	cmd, ok := c.reg.lookup(trimmed)
	if ok && !hasPermission(s.Actor, cmd.Permission) {
		ok = false
	}

	var items []CompletionItem
	add := func(name, hint string) {
		if strings.HasPrefix(name, partial) {
			items = append(items, CompletionItem{Value: name, Label: name, Hint: hint})
		}
	}

	add("--help", helpFlagHint.For(s.Lang))
	add("--json", jsonFlagHint.For(s.Lang))
	if ok && cmd.Destructive {
		add("--yes", yesFlagHint.For(s.Lang))
	}
	if ok {
		for _, f := range cmd.Flags {
			add(f.Name, f.Hint.For(s.Lang))
			if f.Short != "" {
				add(f.Short, f.Hint.For(s.Lang))
			}
		}
	}
	return Completion{From: from, Items: items}
}
