package models

// Anthropic Messages API models (/v1/messages)

// AnthropicRequest is the Anthropic Messages API request
type AnthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []AnthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream,omitempty"`
	// Optional params
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	TopK        int     `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`
}

// AnthropicMessage content can be a string or array of content blocks.
// We support the simple string form and block form.
type AnthropicMessage struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content interface{} `json:"content"`
}

// AnthropicContentBlock is one block of message content
type AnthropicContentBlock struct {
	Type string `json:"type"` // "text"
	Text string `json:"text,omitempty"`
}

// AnthropicResponse is the non-streaming response
type AnthropicResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"` // "message"
	Role         string                  `json:"role"` // "assistant"
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason"` // "end_turn"
	StopSequence *string                 `json:"stop_sequence"`
	Usage        AnthropicUsage          `json:"usage"`
}

// AnthropicUsage
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// --- SSE streaming event payloads (Anthropic format) ---

// AnthropicStreamMessageStart
type AnthropicStreamMessageStart struct {
	Type    string             `json:"type"` // "message_start"
	Message AnthropicStartMsg  `json:"message"`
}

type AnthropicStartMsg struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   *string                 `json:"stop_reason"`
	StopSequence *string                 `json:"stop_sequence"`
	Usage        AnthropicUsage          `json:"usage"`
}

// AnthropicStreamContentBlockStart
type AnthropicStreamContentBlockStart struct {
	Type         string                  `json:"type"` // "content_block_start"
	Index        int                     `json:"index"`
	ContentBlock AnthropicContentBlock   `json:"content_block"`
}

// AnthropicStreamDelta is the delta payload for text_delta events
type AnthropicStreamDelta struct {
	Type  string `json:"type"` // "text_delta"
	Text  string `json:"text"`
}

// AnthropicStreamContentBlockDelta wraps a delta with index
type AnthropicStreamContentBlockDelta struct {
	Type  string `json:"type"` // "content_block_delta"
	Index int    `json:"index"`
	Delta AnthropicStreamDelta `json:"delta"`
}

// AnthropicStreamContentBlockStop
type AnthropicStreamContentBlockStop struct {
	Type  string `json:"type"` // "content_block_stop"
	Index int    `json:"index"`
}

// AnthropicStreamMessageDelta (final usage)
type AnthropicStreamMessageDelta struct {
	Type  string             `json:"type"` // "message_delta"
	Delta AnthropicStopDelta `json:"delta"`
	Usage AnthropicUsage     `json:"usage"`
}

type AnthropicStopDelta struct {
	StopReason   string  `json:"stop_reason"` // "end_turn"
	StopSequence *string `json:"stop_sequence"`
}

// AnthropicStreamMessageStop
type AnthropicStreamMessageStop struct {
	Type string `json:"type"` // "message_stop"
}
