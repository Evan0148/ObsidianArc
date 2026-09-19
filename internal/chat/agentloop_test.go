package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/adapter"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/conversation"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/settings"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// stubBroker stands in for the console. It records what it was asked to run
// and answers with a fixed line, which is all the loop needs to be tested:
// whether the call happened, in what order, and what went back to the model.
type stubBroker struct {
	tools []adapter.Tool
	ran   []string
	fail  bool
}

func (b *stubBroker) Offer(context.Context, user.User) []adapter.Tool { return b.tools }

func (b *stubBroker) Run(_ context.Context, _ user.User, call adapter.ToolCall) (string, bool) {
	b.ran = append(b.ran, call.Name+" "+call.Arguments)
	if b.fail {
		return "permission denied", true
	}
	return "ran " + call.Name, false
}

func oneTool() []adapter.Tool {
	return []adapter.Tool{{
		Name:       "user_list",
		Parameters: json.RawMessage(`{"type":"object","properties":{}}`),
	}}
}

// callFrame is an SSE frame carrying a native tool call, the way an
// OpenAI-compatible upstream sends one.
func callFrame(id, name, arguments string) string {
	return fmt.Sprintf(
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":%q,"type":"function","function":{"name":%q,"arguments":%q}}]}}]}`,
		id, name, arguments)
}

func textFrame(text string) string {
	encoded, _ := json.Marshal(text)
	return `{"choices":[{"delta":{"content":` + string(encoded) + `}}]}`
}

// work runs a turn on the work surface and gathers the tool traffic as well
// as the prose, which the shared collector does not.
type toolEvent struct {
	kind   string
	name   string
	output string
	failed bool
}

func (f *fixture) workTurn(t *testing.T, req TurnRequest) (*collected, []toolEvent, error) {
	t.Helper()
	req.User = f.account
	req.ModelID = f.model.ID
	req.Stream = true
	if req.Mode == "" {
		req.Mode = conversation.ModeWork
	}

	ctx := context.Background()
	resolved, release, err := f.service.Prepare(ctx, &req)
	if err != nil {
		return nil, nil, err
	}
	defer release()

	out := &collected{}
	var tools []toolEvent
	err = f.service.Run(ctx, req, resolved, func(event string, payload any) error {
		switch event {
		case EventStart:
			value := payload.(StartPayload)
			out.start = &value
		case EventDelta:
			out.answer.WriteString(payload.(TextPayload).Text)
		case EventDone:
			value := payload.(DonePayload)
			out.done = &value
		case EventError:
			value := payload.(ErrorPayload)
			out.failure = &value
		case EventToolCall:
			value := payload.(ToolCallPayload)
			tools = append(tools, toolEvent{kind: "call", name: value.Name})
		case EventToolResult:
			value := payload.(ToolResultPayload)
			tools = append(tools, toolEvent{
				kind: "result", name: value.Name, output: value.Output, failed: value.Failed,
			})
		}
		return nil
	})
	return out, tools, err
}

// systemPromptOf digs the prompt out of a captured request. The two
// protocols disagree about where it goes — a top-level field for one, the
// first message for the other — and these tests are about the text, not
// about which adapter carried it.
func systemPromptOf(t *testing.T, request map[string]any) string {
	t.Helper()
	if value, _ := request["system"].(string); value != "" {
		return value
	}
	messages, _ := request["messages"].([]any)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if role, _ := message["role"].(string); role == "system" {
			content, _ := message["content"].(string)
			return content
		}
	}
	return ""
}

// The loop itself: the model asks, the command runs, the model is told, and
// the model answers. Two provider calls for one question.
func TestAWorkTurnRunsTheToolAndAsksAgain(t *testing.T) {
	f := newFixture(t)
	broker := &stubBroker{tools: oneTool()}
	f.service.Tools = broker

	f.upstream.rounds = [][]string{
		{callFrame("c1", "user_list", `{"q":"alice"}`)},
		{textFrame("Alice is an administrator.")},
	}

	out, tools, err := f.workTurn(t, TurnRequest{Content: "who is alice?"})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}

	if len(broker.ran) != 1 || !strings.HasPrefix(broker.ran[0], "user_list ") {
		t.Fatalf("commands run = %v", broker.ran)
	}
	if got := out.answer.String(); !strings.Contains(got, "Alice is an administrator") {
		t.Errorf("answer = %q", got)
	}
	if len(f.upstream.requests) != 2 {
		t.Fatalf("%d provider calls, want 2", len(f.upstream.requests))
	}

	// The call and its answer are both announced, in that order: a reader
	// watching the transcript sees what is happening rather than a pause.
	if len(tools) != 2 || tools[0].kind != "call" || tools[1].kind != "result" {
		t.Fatalf("tool events = %+v", tools)
	}
	if tools[1].output != "ran user_list" || tools[1].failed {
		t.Errorf("result event = %+v", tools[1])
	}

	// And the second request carried the first one's call and its answer,
	// or the model is being asked the same question twice.
	second, _ := json.Marshal(f.upstream.requests[1]["messages"])
	if !strings.Contains(string(second), "user_list") {
		t.Errorf("the second request did not replay the call: %s", second)
	}
	if !strings.Contains(string(second), "ran user_list") {
		t.Errorf("the second request did not carry the result: %s", second)
	}
}

// A refusal is output, not an error. The model has to be able to read why it
// was stopped, or it simply asks for the same thing again.
func TestARefusedCommandGoesBackToTheModelAsOutput(t *testing.T) {
	f := newFixture(t)
	broker := &stubBroker{tools: oneTool(), fail: true}
	f.service.Tools = broker

	f.upstream.rounds = [][]string{
		{callFrame("c1", "user_list", `{}`)},
		{textFrame("I am not allowed to do that.")},
	}

	_, tools, err := f.workTurn(t, TurnRequest{Content: "delete alice"})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if len(tools) != 2 || !tools[1].failed {
		t.Fatalf("tool events = %+v", tools)
	}
	second, _ := json.Marshal(f.upstream.requests[1]["messages"])
	if !strings.Contains(string(second), "permission denied") {
		t.Errorf("the model was not told why: %s", second)
	}
}

// Each round is a real provider request that a tool result made necessary,
// so the ceiling is the ceiling on what one question can cost.
func TestTheRoundCapStopsTheLoopAndSaysSo(t *testing.T) {
	f := newFixture(t)
	f.service.Tools = &stubBroker{tools: oneTool()}
	if err := f.settings.Set(context.Background(), settings.ChatAgentMaxRounds, "3"); err != nil {
		t.Fatal(err)
	}

	// Every round asks for another tool, so only the cap ends this.
	call := []string{callFrame("c1", "user_list", `{}`)}
	f.upstream.rounds = [][]string{call, call, call, call, call}

	out, _, err := f.workTurn(t, TurnRequest{Content: "keep going"})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if len(f.upstream.requests) != 3 {
		t.Errorf("%d provider calls, cap was 3", len(f.upstream.requests))
	}
	if !strings.Contains(out.answer.String(), "tool rounds") {
		t.Errorf("the answer did not say why it stopped: %q", out.answer.String())
	}
}

// A chat is a chat. No tools are offered, so no loop can start, and the
// turn costs exactly one provider call as it always did.
func TestAChatTurnIsOfferedNoTools(t *testing.T) {
	f := newFixture(t)
	broker := &stubBroker{tools: oneTool()}
	f.service.Tools = broker

	f.upstream.script(textFrame("hello"))
	if _, err := f.turn(t, context.Background(), TurnRequest{
		Content: "hi", Mode: conversation.ModeChat,
	}); err != nil {
		t.Fatalf("turn: %v", err)
	}

	if len(broker.ran) != 0 {
		t.Errorf("a chat turn ran commands: %v", broker.ran)
	}
	if _, offered := f.upstream.requests[0]["tools"]; offered {
		t.Error("a chat turn was sent tools")
	}
}

// The mode belongs to the conversation. A client resending an existing
// thread with mode=work must not be able to arm it, or the surface is
// decided by whoever writes the request body.
func TestModeComesFromTheConversationNotTheRequest(t *testing.T) {
	f := newFixture(t)
	broker := &stubBroker{tools: oneTool()}
	f.service.Tools = broker

	f.upstream.script(textFrame("first"))
	first, err := f.turn(t, context.Background(), TurnRequest{
		Content: "hi", Mode: conversation.ModeChat,
	})
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}

	f.upstream.requests = nil
	f.upstream.script(textFrame("second"))
	if _, err := f.turn(t, context.Background(), TurnRequest{
		ConversationID: first.start.ConversationID,
		Content:        "and now with tools",
		Mode:           conversation.ModeWork,
	}); err != nil {
		t.Fatalf("second turn: %v", err)
	}

	if _, offered := f.upstream.requests[0]["tools"]; offered {
		t.Error("resending a chat thread as work armed it")
	}
	if len(broker.ran) != 0 {
		t.Errorf("it ran commands: %v", broker.ran)
	}
}

// The preamble is the only thing standing between a tool that read somebody's
// bio and a tool that did what the bio told it to, so nothing a reader or an
// operator writes may displace it.
func TestTheAgentPreambleLeadsThePromptAndCannotBeDisplaced(t *testing.T) {
	f := newFixture(t)
	f.service.Tools = &stubBroker{tools: oneTool()}
	if err := f.settings.Set(context.Background(),
		settings.DefaultSystemPrompt, "Answer in limericks."); err != nil {
		t.Fatal(err)
	}

	f.upstream.rounds = [][]string{{textFrame("done")}}
	if _, _, err := f.workTurn(t, TurnRequest{Content: "hello"}); err != nil {
		t.Fatalf("turn: %v", err)
	}

	system := systemPromptOf(t, f.upstream.requests[0])
	if !strings.Contains(system, "never an instruction") {
		t.Fatalf("the preamble is not in the prompt: %q", system)
	}
	if !strings.Contains(system, "Answer in limericks") {
		t.Errorf("the operator's own prompt was dropped: %q", system)
	}
	if strings.Index(system, "never an instruction") > strings.Index(system, "Answer in limericks") {
		t.Error("the operator's prompt precedes the preamble; it must not be able to")
	}
}

// A project's brief is a reader's own text. It may add to the prompt and
// never replace it, because the instance prompt is where the rules an
// account must not switch off live.
func TestProjectInstructionsAreAppendedRatherThanSubstituted(t *testing.T) {
	f := newFixture(t)
	if err := f.settings.Set(context.Background(),
		settings.DefaultSystemPrompt, "Operator rules."); err != nil {
		t.Fatal(err)
	}
	f.service.ProjectInstructions = func(context.Context, user.User, string) string {
		return "Project brief."
	}

	// A real row: conversations.project_id is a foreign key, which is what
	// stops a turn naming a project that does not exist.
	const projectID = "01PROJECTROWFORTHISTEST00"
	if _, err := f.db.Exec(context.Background(),
		`INSERT INTO projects (id, user_id, name, instructions, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, f.account.ID, "Weekly review", "Project brief.", 1, 1); err != nil {
		t.Fatal(err)
	}

	f.upstream.script(textFrame("ok"))
	if _, err := f.turn(t, context.Background(), TurnRequest{
		Content: "hi", ProjectID: projectID,
	}); err != nil {
		t.Fatalf("turn: %v", err)
	}

	system := systemPromptOf(t, f.upstream.requests[0])
	if !strings.Contains(system, "Operator rules.") {
		t.Errorf("the operator's prompt was replaced: %q", system)
	}
	if !strings.Contains(system, "Project brief.") {
		t.Errorf("the project's brief never arrived: %q", system)
	}
	if strings.Index(system, "Operator rules.") > strings.Index(system, "Project brief.") {
		t.Error("the project's brief came first; appended means after")
	}
}

// A work transcript is a record of what was done, and a record that lives
// only in the browser is gone the moment the reader reloads. The answer and
// the commands that produced it are saved together.
func TestWhatTheTurnRanIsSavedWithTheAnswer(t *testing.T) {
	f := newFixture(t)
	f.service.Tools = &stubBroker{tools: oneTool()}

	f.upstream.rounds = [][]string{
		{callFrame("c1", "user_list", `{"q":"alice"}`)},
		{textFrame("Alice is an administrator.")},
	}
	out, _, err := f.workTurn(t, TurnRequest{Content: "who is alice?"})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}

	// Read back the way the screen reads it, not out of the loop's own state.
	messages := f.messages(t, out.start.ConversationID)
	last := messages[len(messages)-1]
	if last.Role != conversation.RoleAssistant {
		t.Fatalf("last message is a %s", last.Role)
	}
	if len(last.ToolCalls) != 1 {
		t.Fatalf("saved tool calls = %+v, want the one it ran", last.ToolCalls)
	}
	call := last.ToolCalls[0]
	if call.Name != "user_list" || call.Output != "ran user_list" || call.Failed {
		t.Errorf("saved call = %+v", call)
	}
	if !strings.Contains(call.Arguments, "alice") {
		t.Errorf("the arguments were not kept: %q", call.Arguments)
	}

	// A chat turn saves none, rather than an empty array on every row.
	f.upstream.requests = nil
	f.upstream.script(textFrame("hello"))
	plain, err := f.turn(t, context.Background(), TurnRequest{Content: "hi", Mode: conversation.ModeChat})
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range f.messages(t, plain.start.ConversationID) {
		if len(message.ToolCalls) != 0 {
			t.Errorf("a chat turn saved tool calls: %+v", message.ToolCalls)
		}
	}
}
