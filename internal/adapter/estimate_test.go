package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func userSays(text string) ChatRequest {
	return ChatRequest{Model: testModel(), Stream: true, Messages: []Message{
		{Role: RoleUser, Parts: []Part{{Kind: PartText, Text: text}}},
	}}
}

// A stream that finishes without a usage frame is estimated from what went
// out and what came back, and marked so the ledger can say so.
func TestASilentProviderIsEstimated(t *testing.T) {
	server := sseServer(t, []string{
		`{"choices":[{"delta":{"content":"` + strings.Repeat("abcd", 50) + `"}}]}`,
	}, nil)

	var out collected
	result, err := testRegistry().Chat(context.Background(),
		Provider{Kind: KindOpenAI, BaseURL: server.URL, APIKey: "k"},
		userSays(strings.Repeat("wxyz", 100)), out.sink)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if !result.Usage.Estimated {
		t.Fatal("a provider that reported nothing was not estimated")
	}
	// Four hundred bytes asked and two hundred answered, at four to the
	// token, plus the framing allowance on the one message.
	if result.Usage.InputTokens != 100+perMessageTax || result.Usage.OutputTokens != 50 {
		t.Errorf("usage = %+v, want %d in and 50 out", result.Usage, 100+perMessageTax)
	}
}

// Some adapters report only through events. An estimate written into the
// result would be merged over those counts by every caller, replacing a real
// number with a guess.
func TestUsageReportedInTheStreamIsLeftAlone(t *testing.T) {
	server := sseServer(t, []string{
		`{"choices":[{"delta":{"content":"done"}}]}`,
		`{"usage":{"prompt_tokens":3,"completion_tokens":2}}`,
	}, nil)

	var out collected
	result, err := testRegistry().Chat(context.Background(),
		Provider{Kind: KindOpenAI, BaseURL: server.URL, APIKey: "k"},
		userSays("hi"), out.sink)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if result.Usage.Estimated {
		t.Errorf("reported usage was replaced with an estimate: %+v", result.Usage)
	}
	for _, usage := range out.usages {
		if usage.Estimated {
			t.Errorf("an estimate reached the stream: %+v", usage)
		}
	}
}

// A call that failed is not estimated. What a failed or stopped turn costs is
// decided elsewhere, and guessing at half an answer here would change it.
func TestAFailedCallIsNotEstimated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)

	var out collected
	result, err := testRegistry().Chat(context.Background(),
		Provider{Kind: KindOpenAI, BaseURL: server.URL, APIKey: "k"},
		userSays("hi"), out.sink)
	if err == nil {
		t.Fatal("a 502 was not reported as a failure")
	}
	if result.Usage.Total() != 0 || result.Usage.Estimated {
		t.Errorf("a failed call carried usage %+v", result.Usage)
	}
}

// Once one round of a turn is a guess, the turn's total is too.
func TestMergingAnEstimateKeepsTheMark(t *testing.T) {
	merged := Usage{InputTokens: 10, OutputTokens: 5}.Merge(Usage{InputTokens: 20, OutputTokens: 8, Estimated: true})
	if !merged.Estimated {
		t.Error("the mark was lost in the merge")
	}
}
