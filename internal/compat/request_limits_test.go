package compat

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestLongToolHistoryReachesTheProvider(t *testing.T) {
	cases := []struct {
		path   string
		field  string
		call   string
		result string
	}{
		{
			path:   "/v1/chat/completions",
			field:  "messages",
			call:   `{"role":"assistant","tool_calls":[{"id":"call_%[1]d","type":"function","function":{"name":"read_file","arguments":"{\"index\":%[1]d}"}}]}`,
			result: `{"role":"tool","tool_call_id":"call_%[1]d","content":"result_%[1]d"}`,
		},
		{
			path:   "/v1/messages",
			field:  "messages",
			call:   `{"role":"assistant","content":[{"type":"tool_use","id":"call_%[1]d","name":"read_file","input":{"index":%[1]d}}]}`,
			result: `{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_%[1]d","content":"result_%[1]d"}]}`,
		},
		{
			path:   "/v1/responses",
			field:  "input",
			call:   `{"type":"function_call","call_id":"call_%[1]d","name":"read_file","arguments":"{\"index\":%[1]d}"}`,
			result: `{"type":"function_call_output","call_id":"call_%[1]d","output":"result_%[1]d"}`,
		},
	}
	for _, c := range cases {
		// Tool loops can cross the old 400-item boundary long before the
		// transcript fills the model's context or the request body allowance.
		for _, rounds := range []int{200, 500} {
			t.Run(fmt.Sprintf("%s/%d_items", c.path, 1+2*rounds), func(t *testing.T) {
				f := newFixture(t)
				f.upstream.reply(answer)

				items := []string{`{"role":"user","content":"start"}`}
				for i := 0; i < rounds; i++ {
					items = append(items, fmt.Sprintf(c.call, i), fmt.Sprintf(c.result, i))
				}
				body := fmt.Sprintf(`{"model":%q,"%s":[%s]}`, f.model.ID, c.field, strings.Join(items, ","))
				w := f.do(t, http.MethodPost, c.path, f.token, body)
				if w.Code != http.StatusOK {
					t.Fatalf("status = %d: %s", w.Code, w.Body.String())
				}

				sent, _ := f.upstream.received()["messages"].([]any)
				if len(sent) != len(items) {
					t.Fatalf("provider received %d messages, want %d", len(sent), len(items))
				}
				first, _ := sent[0].(map[string]any)
				if first["role"] != "user" || first["content"] != "start" {
					t.Fatalf("initial message = %v", first)
				}
				for i := 0; i < rounds; i++ {
					assistant, _ := sent[1+2*i].(map[string]any)
					calls, _ := assistant["tool_calls"].([]any)
					if assistant["role"] != "assistant" || len(calls) != 1 {
						t.Fatalf("round %d: assistant message = %v", i, assistant)
					}
					call, _ := calls[0].(map[string]any)
					function, _ := call["function"].(map[string]any)
					callID := fmt.Sprintf("call_%d", i)
					if call["id"] != callID || function["name"] != "read_file" ||
						function["arguments"] != fmt.Sprintf(`{"index":%d}`, i) {
						t.Fatalf("round %d: tool call = %v", i, call)
					}
					result, _ := sent[2+2*i].(map[string]any)
					if result["role"] != "tool" || result["tool_call_id"] != callID ||
						result["content"] != fmt.Sprintf("result_%d", i) {
						t.Fatalf("round %d: tool result = %v", i, result)
					}
				}

				if c.path == "/v1/messages" {
					count := f.do(t, http.MethodPost, "/v1/messages/count_tokens", f.token, body)
					if count.Code != http.StatusOK {
						t.Fatalf("count_tokens status = %d: %s", count.Code, count.Body.String())
					}
					tokens, _ := decodeJSON(t, count)["input_tokens"].(float64)
					if tokens <= 0 {
						t.Fatalf("count_tokens input_tokens = %v", tokens)
					}
				}
			})
		}
	}
}

func TestTranscriptBodySizeIsStillBounded(t *testing.T) {
	for _, path := range []string{
		"/v1/chat/completions",
		"/v1/messages",
		"/v1/messages/count_tokens",
		"/v1/responses",
	} {
		t.Run(path, func(t *testing.T) {
			f := newFixture(t)
			field := "messages"
			if path == "/v1/responses" {
				field = "input"
			}
			body := fmt.Sprintf(`{"model":%q,"%s":[{"role":"user","content":"%s"}]}`,
				f.model.ID, field, strings.Repeat("x", maxBodyBytes))
			w := f.do(t, http.MethodPost, path, f.token, body)
			if w.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("status = %d, want 413: %s", w.Code, w.Body.String())
			}
			if f.upstream.received() != nil {
				t.Fatal("oversized request reached the provider")
			}
		})
	}
}
