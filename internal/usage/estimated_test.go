package usage

import (
	"context"
	"testing"
)

// The mark survives the ledger in both directions: an estimated turn reads
// back as one, a counted turn reads back as not, and the account's own view
// carries it too — that is where somebody compares the figure against what
// they expected.
func TestTheLedgerKeepsWhichFiguresWereEstimated(t *testing.T) {
	store, account := seriesFixture(t)
	ctx := context.Background()

	for _, record := range []Record{
		{RequestID: "counted", InputTokens: 11, OutputTokens: 7, TotalTokens: 18, StartedAt: 1, FinishedAt: 2},
		{RequestID: "guessed", InputTokens: 40, OutputTokens: 9, TotalTokens: 49, Estimated: true, StartedAt: 3, FinishedAt: 4},
	} {
		record.UserID, record.GroupID, record.Status = account.ID, account.GroupID, StatusOK
		if err := store.Write(ctx, record); err != nil {
			t.Fatal(err)
		}
	}

	rows, _, err := store.List(ctx, Filter{UserID: account.ID})
	if err != nil {
		t.Fatal(err)
	}
	marks := map[string]bool{}
	for _, row := range rows {
		marks[row.RequestID] = row.Estimated
		if toPublic(row).Estimated != row.Estimated {
			t.Errorf("the account's view of %s lost the mark", row.RequestID)
		}
	}
	if marks["counted"] || !marks["guessed"] {
		t.Errorf("marks = %v, want only the guessed turn estimated", marks)
	}
}
