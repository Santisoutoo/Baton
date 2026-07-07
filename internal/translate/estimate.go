package translate

import "encoding/json"

// EstimateInputTokens gives a rough token count for an Anthropic request body,
// used to answer /v1/messages/count_tokens on the OpenAI lane (which has no such
// endpoint). It sums the length of all text we can see and divides by ~4 chars
// per token — a deliberately cheap approximation, not a real tokenizer.
func EstimateInputTokens(body []byte) int {
	var req AnthropicRequest
	if json.Unmarshal(body, &req) != nil {
		return len(body) / 4
	}
	chars := len(systemText(req.System))
	for _, m := range req.Messages {
		if s, ok := asString(m.Content); ok {
			chars += len(s)
			continue
		}
		var blocks []ContentBlock
		if json.Unmarshal(m.Content, &blocks) == nil {
			for _, b := range blocks {
				chars += len(b.Text)
				chars += len(b.Input)
				chars += len(b.Content)
			}
		}
	}
	tokens := chars / 4
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}
