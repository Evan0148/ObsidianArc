package conversation

import (
	"context"
	"sync"
	"testing"
)

// Two turns on one conversation used to read the same MAX(seq) and choose the
// same position for both messages. ux_messages_seq then failed the loser, so a
// turn vanished with an error neither reader caused — and because the append
// returns before the usage is recorded, the tokens were spent and never
// reached the ledger. Real goroutines, because that is what this repository
// asks of a concurrency fix.
func TestConcurrentAppendsTakeDistinctPositions(t *testing.T) {
	store, _, account := attachmentFixture(t)
	ctx := context.Background()

	thread, err := store.Create(ctx, nil, account.ID, NewConversation{Title: "One conversation"})
	if err != nil {
		t.Fatal(err)
	}

	const writers = 8
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		seen = map[int]bool{}
		errs []error
	)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			message, err := store.Append(ctx, nil, AppendInput{
				ConversationID: thread.ID,
				UserID:         account.ID,
				Role:           RoleUser,
				Content:        "hello",
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			if seen[message.Seq] {
				t.Errorf("two messages were given position %d", message.Seq)
			}
			seen[message.Seq] = true
		}()
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("%d of %d appends failed, first: %v", len(errs), writers, errs[0])
	}
	if len(seen) != writers {
		t.Fatalf("%d distinct positions for %d messages", len(seen), writers)
	}
	// Consecutive from one. Editing and regenerating leaves gaps by design,
	// but nothing here was deleted.
	for position := 1; position <= writers; position++ {
		if !seen[position] {
			t.Errorf("no message at position %d: %v", position, seen)
		}
	}

	// And the stored transcript agrees with what was appended.
	messages, err := store.Messages(ctx, nil, account.ID, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != writers {
		t.Errorf("stored %d messages, want %d", len(messages), writers)
	}
}
