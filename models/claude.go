package models

import "encoding/json"

// Claude.ai internal API models (reverse-engineered via js-reverse)

// ClaudeOrganization represents an org from claude.ai
type ClaudeOrganization struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// ClaudeOrganizationsResponse is the /api/organizations response
type ClaudeOrganizationsResponse []ClaudeOrganization

// ClaudeConversation represents a conversation
type ClaudeConversation struct {
	UUID             string              `json:"uuid"`
	Name             string              `json:"name"`
	CreatedAt        string              `json:"created_at"`
	UpdatedAt        string              `json:"updated_at"`
	OrganizationUUID string              `json:"organization_uuid"`
	Summary          string              `json:"summary,omitempty"`
	ChatMessages     []ClaudeChatMessage `json:"chat_messages,omitempty"`
}

// ClaudeCreateConversationRequest creates a new conversation
type ClaudeCreateConversationRequest struct {
	Name             string `json:"name"`
	OrganizationUUID string `json:"organization_uuid,omitempty"`
}

// ClaudeChatMessage represents a message in a conversation
type ClaudeChatMessage struct {
	UUID        string             `json:"uuid"`
	Text        string             `json:"text"`
	Sender      string             `json:"sender"` // "human" or "assistant"
	CreatedAt   string             `json:"created_at"`
	Attachments []ClaudeAttachment `json:"attachments,omitempty"`
	Files       []ClaudeFile       `json:"files,omitempty"`
}

// ClaudeAttachment is an attachment in claude.ai's format
type ClaudeAttachment struct {
	FileName         string `json:"file_name"`
	FileType         string `json:"file_type"`
	FileSize         int    `json:"file_size"`
	ExtractedContent string `json:"extracted_content,omitempty"`
}

// ClaudeFile is a file uploaded to claude.ai
type ClaudeFile struct {
	FileName   string `json:"file_name"`
	FileType   string `json:"file_type"`
	FileSize   int    `json:"file_size"`
	FileBase64 string `json:"file_base64,omitempty"`
}

// ClaudeCompletionRequest is the body sent to /completion endpoint
// Based on real captured request from js-reverse
type ClaudeCompletionRequest struct {
	Prompt                   string                    `json:"prompt"`
	ParentMessageUUID        string                    `json:"parent_message_uuid,omitempty"`
	Timezone                 string                    `json:"timezone"`
	Locale                   string                    `json:"locale"`
	Model                    string                    `json:"model"`
	Effort                   string                    `json:"effort,omitempty"`        // "low"|"medium"|"high"|"xhigh"|"max"
	ThinkingMode             string                    `json:"thinking_mode,omitempty"` // "auto"|"none"
	Tools                    json.RawMessage           `json:"tools,omitempty"`
	TurnMessageUUIDs         *TurnMessageUUIDs         `json:"turn_message_uuids,omitempty"`
	Attachments              []ClaudeAttachment        `json:"attachments"`
	Files                    []ClaudeFile              `json:"files"`
	SyncSources              []interface{}             `json:"sync_sources"`
	RenderingMode            string                    `json:"rendering_mode"`
	CreateConversationParams *CreateConversationParams `json:"create_conversation_params,omitempty"`
}

// CreateConversationParams mirrors claude.ai web create-conversation options bundled with completion.
type CreateConversationParams struct {
	Name                           string      `json:"name"`
	Model                          string      `json:"model"`
	IncludeConversationPreferences bool        `json:"include_conversation_preferences"`
	PaprikaMode                    interface{} `json:"paprika_mode"`
	CompassMode                    interface{} `json:"compass_mode"`
	ToolSearchMode                 string      `json:"tool_search_mode"`
	IsTemporary                    bool        `json:"is_temporary"`
	EnabledImagine                 bool        `json:"enabled_imagine"`
}

// TurnMessageUUIDs holds the human/assistant message UUIDs for the turn
type TurnMessageUUIDs struct {
	HumanMessageUUID     string `json:"human_message_uuid"`
	AssistantMessageUUID string `json:"assistant_message_uuid"`
}

// ClaudeCompletionEvent is a single SSE event from the completion endpoint
type ClaudeCompletionEvent struct {
	Type string `json:"type"`
	Data string `json:"-"`
	// message_start
	Message *ClaudeCompletionMessage `json:"message,omitempty"`
	// content_block_start / content_block_stop
	Index        int           `json:"index,omitempty"`
	ContentBlock *ContentBlock `json:"content_block,omitempty"`
	// content_block_delta / message_delta share the same raw "delta" field.
	Delta json.RawMessage `json:"delta,omitempty"`
	// Parsed text delta for content_block_delta events.
	TextDelta *ClaudeCompletionDelta `json:"-"`
	// Parsed stop delta for message_delta events.
	MessageDelta *MessageDeltaPayload `json:"-"`
	// content_block_stop
	StopTimestamp string `json:"stop_timestamp,omitempty"`
	// message_limit
	MessageLimit *MessageLimitPayload `json:"message_limit,omitempty"`
	// error
	Error *ClaudeError `json:"error,omitempty"`
}

// ClaudeCompletionMessage from message_start event
type ClaudeCompletionMessage struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	Model        string `json:"model"`
	ParentUUID   string `json:"parent_uuid"`
	UUID         string `json:"uuid"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	StopDetails  string `json:"stop_details"`
	TraceID      string `json:"trace_id"`
	RequestID    string `json:"request_id"`
}

// ContentBlock from content_block_start event
type ContentBlock struct {
	Type           string        `json:"type"` // "text"
	Text           string        `json:"text"`
	Citations      []interface{} `json:"citations"`
	StartTimestamp string        `json:"start_timestamp"`
	StopTimestamp  string        `json:"stop_timestamp"`
}

// ClaudeCompletionDelta from content_block_delta event
type ClaudeCompletionDelta struct {
	Type    string `json:"type"` // "text_delta" | "thinking_delta"
	Text    string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`
}

// MessageDeltaPayload from message_delta event (final stop info)
type MessageDeltaPayload struct {
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	StopDetails  string `json:"stop_details"`
}

// MessageLimitPayload from message_limit event
type MessageLimitPayload struct {
	Type string `json:"type"`
}

// ClaudeError represents an error response
type ClaudeError struct {
	ErrorType string `json:"error_type"`
	Message   string `json:"message"`
}

// ClaudeConversationsResponse is the list conversations response
type ClaudeConversationsResponse struct {
	Results []ClaudeConversation `json:"results"`
	HasMore bool                 `json:"has_more"`
	Total   int                  `json:"total"`
}
