package models

// OpenAI Responses API models (/v1/responses)

// ResponsesRequest is the OpenAI Responses API request
type ResponsesRequest struct {
	Model       string           `json:"model"`
	Input       ResponseInput    `json:"input"` // string or array of input items
	Instructions string          `json:"instructions,omitempty"` // system prompt
	Stream      bool             `json:"stream,omitempty"`
	MaxOutputTokens int          `json:"max_output_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
}

// ResponseInput can be a string or an array of items. Use interface{}.
type ResponseInput interface{}

// ResponseInputItem is one item in a structured input array
type ResponseInputItem struct {
	Type    string                  `json:"type"`    // "message"
	Role    string                  `json:"role"`    // "user" | "assistant" | "system" | "developer"
	Content ResponseItemContent     `json:"content"` // string or array of parts
}

// ResponseItemContent can be string or []part
type ResponseItemContent interface{}

// ResponsesResponse is the non-streaming response
type ResponsesResponse struct {
	ID        string             `json:"id"`
	Object    string             `json:"object"` // "response"
	CreatedAt int64              `json:"created_at"`
	Model     string             `json:"model"`
	Status    string             `json:"status"` // "completed"
	Output    []ResponseOutputItem `json:"output"`
	Usage     ResponsesUsage     `json:"usage"`
}

// ResponseOutputItem is one output item (a message)
type ResponseOutputItem struct {
	Type    string                 `json:"type"` // "message"
	ID      string                 `json:"id"`
	Role    string                 `json:"role"` // "assistant"
	Status  string                 `json:"status"` // "completed"
	Content []ResponseContentPart  `json:"content"`
}

// ResponseContentPart
type ResponseContentPart struct {
	Type string `json:"type"` // "output_text"
	Text string `json:"text"`
}

// ResponsesUsage
type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// --- SSE streaming events (Responses API) ---

// ResponsesStreamEvent is a generic SSE event for the Responses API
type ResponsesStreamEvent struct {
	Type    string      `json:"type"`
	// Fields vary by type; we use the specific structs below per event.
}

// ResponsesResponseCreated
type ResponsesResponseCreated struct {
	Type     string             `json:"type"` // "response.created"
	Response ResponsesResponse `json:"response"`
}

// ResponsesOutputItemAdded
type ResponsesOutputItemAdded struct {
	Type         string               `json:"type"` // "response.output_item.added"
	OutputIndex int                  `json:"output_index"`
	Item        ResponseOutputItem   `json:"item"`
}

// ResponsesContentPartAdded
type ResponsesContentPartAdded struct {
	Type         string               `json:"type"` // "response.content_part.added"
	ItemID       string               `json:"item_id"`
	OutputIndex  int                  `json:"output_index"`
	ContentIndex int                  `json:"content_index"`
	Part         ResponseContentPart  `json:"part"`
}

// ResponsesOutputTextDelta
type ResponsesOutputTextDelta struct {
	Type         string `json:"type"` // "response.output_text.delta"
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

// ResponsesContentPartDone
type ResponsesContentPartDone struct {
	Type         string               `json:"type"` // "response.content_part.done"
	ItemID       string               `json:"item_id"`
	OutputIndex  int                   `json:"output_index"`
	ContentIndex int                   `json:"content_index"`
	Part         ResponseContentPart   `json:"part"`
}

// ResponsesOutputItemDone
type ResponsesOutputItemDone struct {
	Type         string               `json:"type"` // "response.output_item.done"
	OutputIndex int                  `json:"output_index"`
	Item        ResponseOutputItem    `json:"item"`
}

// ResponsesCompleted
type ResponsesCompleted struct {
	Type     string             `json:"type"` // "response.completed"
	Response ResponsesResponse `json:"response"`
}
