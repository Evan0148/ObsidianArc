package adapter

// Token estimates, for the two places a real count is not available.
//
// The first is a client asking before it sends: /v1/messages/count_tokens,
// which agent clients call to decide when to compact. The second is a
// provider that answered and never said what it cost. Several gateways in
// front of web products do exactly that for some of their models, and the
// ledger then carried a turn that plainly happened as zero tokens — which
// read, to an operator whose free models all sat on such a gateway, as
// "a price of zero makes the tokens zero".
//
// It is an estimate and cannot be anything else. A real count needs the
// tokeniser of the model that answered, which differs per family, and
// carrying one would be both a dependency this project does not take and a
// second thing to keep in step with every model an operator adds. Four bytes
// to the token is the usual approximation: within a fifth or so for English
// and for code, and for CJK it comes out at three quarters of a token per
// character, because each character is three bytes in UTF-8.

const (
	bytesPerToken = 4
	// A small allowance for the framing every message and every tool call
	// carries on the wire.
	perMessageTax  = 4
	perToolCallTax = 8
	// Rather than the bytes, which are base64 and say nothing about how the
	// model will see the picture.
	imageTokenGuess = 1600
)

// EstimatePrompt is roughly what a request costs to read.
func EstimatePrompt(request ChatRequest) int {
	bytes := len(request.System)

	for _, tool := range request.Tools {
		// An agent's tool definitions are often most of its prompt, so
		// leaving them out would understate the total badly.
		bytes += len(tool.Name) + len(tool.Description) + len(tool.Parameters)
	}

	tokens := 0
	for _, message := range request.Messages {
		tokens += perMessageTax
		for _, part := range message.Parts {
			switch part.Kind {
			case PartImage:
				tokens += imageTokenGuess
			case PartToolCall:
				bytes += len(part.ToolName) + len(part.ToolArgs)
				tokens += perToolCallTax
			default:
				bytes += len(part.Text) + len(part.ToolCallID)
			}
		}
	}

	return tokens + bytes/bytesPerToken
}

// estimateResult is roughly what an answer cost, given the request it
// answered. Marked, so nothing downstream can mistake it for a figure the
// provider reported.
func estimateResult(request ChatRequest, result Result) Usage {
	output := len(result.Text) / bytesPerToken
	for _, call := range result.ToolCalls {
		output += perToolCallTax + (len(call.Name)+len(call.Arguments))/bytesPerToken
	}
	return Usage{
		InputTokens:     EstimatePrompt(request),
		OutputTokens:    output,
		ReasoningTokens: len(result.Reasoning) / bytesPerToken,
		Estimated:       true,
	}
}
