package usage

import (
	"context"
	"fmt"
	"testing"
)

func TestBreakdownIncludesRowsBeyondFifty(t *testing.T) {
	store, account := seriesFixture(t)
	ctx := context.Background()
	for i := 0; i < 65; i++ {
		if err := store.Write(ctx, Record{UserID: account.ID, GroupID: account.GroupID,
			ModelID: fmt.Sprintf("model-%02d", i), ModelName: fmt.Sprintf("Model %02d", i),
			RequestID: fmt.Sprintf("r-%d", i), Status: StatusOK, Credits: float64(i), StartedAt: int64(i), FinishedAt: int64(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.GroupBy(ctx, "model", MetricCredits, Filter{})
	if err != nil || len(rows) != 65 {
		t.Fatalf("breakdown: %d rows, %v", len(rows), err)
	}
	if rows[0].Key != "model-64" || rows[64].Key != "model-00" {
		t.Fatal("breakdown ranking lost")
	}
	last, total, err := store.List(ctx, Filter{Limit: 20, Offset: 60})
	if err != nil || total != 65 || len(last) != 5 {
		t.Fatalf("last page: %d/%d, %v", len(last), total, err)
	}
}
