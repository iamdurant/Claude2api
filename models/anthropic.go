package models

// Anthropic Messages API models (/v1/messages)

// AnthropicTool is a tool definition supplied by Claude Code.
type AnthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
}

// AnthropicRequest is the Anthropic Messages API request
type AnthropicRequest struct {
	Model          string             `json:"model"`
	Messages       []AnthropicMessage `json:"messages"`
	System         interface{}        `json:"system,omitempty"`
	MaxTokens      int                `json:"max_tokens"`
	Stream         bool               `json:"stream,omitempty"`
	ConversationID string             `json:"conversation_id,omitempty"`
	ToolDefs       []AnthropicTool    `json:"tools,omitempty"`
	Thinking       interface{}        `json:"thinking,omitempty"`
	// Optional params
	Temperature   float64  `json:"temperature,omitempty"`
	TopP          float64  `json:"top_p,omitempty"`
	TopK          int      `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`
}

// AnthropicMessage content can be a string or array of content blocks.
type AnthropicMessage struct {
	Role    string      `json:"role"` // "user" or "assistant"
	Content interface{} `json:"content"`
}

// AnthropicContentBlock is one block of message content.
// type can be "text" | "tool_use" | "tool_result".
type AnthropicContentBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text,omitempty"`
	ID           string                 `json:"id,omitempty"`
	Name         string                 `json:"name,omitempty"`
	Input        map[string]interface{} `json:"input,omitempty"`
	Content      interface{}            `json:"content,omitempty"`
	IsError      *bool                  `json:"is_error,omitempty"`
	UseID        string                 `json:"use_id,omitempty"`
	CacheControl interface{}            `json:"cache_control,omitempty"`
}

// AnthropicResponse is the non-streaming response
type AnthropicResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"` // "message"
	Role         string                  `json:"role"` // "assistant"
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason"` // "end_turn" | "tool_use"
	StopSequence *string                 `json:"stop_sequence"`
	Usage        AnthropicUsage          `json:"usage"`
}

// AnthropicUsage
type AnthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// --- SSE streaming event payloads (Anthropic format) ---

// AnthropicStreamMessageStart
type AnthropicStreamMessageStart struct {
	Type    string            `json:"type"` // "message_start"
	Message AnthropicStartMsg `json:"message"`
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
	Type         string                `json:"type"` // "content_block_start"
	Index        int                   `json:"index"`
	ContentBlock AnthropicContentBlock `json:"content_block"`
}

// AnthropicStreamDelta is the delta payload for text_delta events
type AnthropicStreamDelta struct {
	Type string `json:"type"` // "text_delta" | "input_json_delta"
	Text string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

// AnthropicStreamContentBlockDelta wraps a delta with index
type AnthropicStreamContentBlockDelta struct {
	Type  string               `json:"type"` // "content_block_delta"
	Index int                  `json:"index"`
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
	StopReason   string  `json:"stop_reason"` // "end_turn" | "tool_use"
	StopSequence *string `json:"stop_sequence"`
}

// AnthropicStreamMessageStop
type AnthropicStreamMessageStop struct {
	Type string `json:"type"` // "message_stop"
}
