package handlers

import (
	"net/http"
	"strings"

	"claude2api/claude"
	"claude2api/models"

	"github.com/gin-gonic/gin"
)

// AnthropicMessages handles POST /v1/messages (Anthropic format, streaming + non-streaming)
func (h *Handler) AnthropicMessages(c *gin.Context) {
	var req models.AnthropicRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid request body: "+err.Error())
		return
	}
	if len(req.Messages) == 0 {
		badRequest(c, "messages must contain at least one message")
		return
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 4096
	}

	claudeModel, err := resolveModel(req.Model, h.cfg.DefaultModel)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	client, err := h.newClient(c)
	if err != nil {
		internalError(c, "create client: "+err.Error())
		return
	}

	prompt := buildAnthropicPrompt(req)
	effort := "medium"

	if req.Stream {
		h.anthropicStream(c, client, prompt, claudeModel, effort, req.ConversationID)
	} else {
		h.anthropicNonStream(c, client, prompt, claudeModel, effort, req.ConversationID)
	}
}

// buildAnthropicPrompt converts Anthropic messages into the single-prompt format
func buildAnthropicPrompt(req models.AnthropicRequest) string {
	var parts []string
	if req.System != "" {
		parts = append(parts, "[System]\n"+req.System)
	}
	for _, m := range req.Messages {
		text := anthropicContentToString(m.Content)
		switch m.Role {
		case "user":
			parts = append(parts, "[Human]\n"+text)
		case "assistant":
			parts = append(parts, "[Assistant]\n"+text)
		default:
			parts = append(parts, text)
		}
	}
	parts = append(parts, "[Assistant]\n")
	return strings.Join(parts, "\n\n")
}

// anthropicContentToString flattens a message content (string or []block) to text
func anthropicContentToString(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var sb strings.Builder
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if t, _ := m["type"].(string); t == "text" {
					if s, _ := m["text"].(string); s != "" {
						sb.WriteString(s)
					}
				}
			}
		}
		return sb.String()
	default:
		return ""
	}
}

func (h *Handler) anthropicNonStream(c *gin.Context, client *claude.Client, prompt, claudeModel, effort, conversationID string) {
	content, err := h.runCompletion(c.Request.Context(), client, prompt, claudeModel, effort, conversationID, nil)
	if err != nil {
		upstreamError(c, err.Error())
		return
	}
	resp := models.AnthropicResponse{
		ID:         genID("msg_"),
		Type:       "message",
		Role:       "assistant",
		Content:    []models.AnthropicContentBlock{{Type: "text", Text: content}},
		Model:      claudeModel,
		StopReason: "end_turn",
		Usage: models.AnthropicUsage{
			OutputTokens: len(content) / 4,
		},
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) anthropicStream(c *gin.Context, client *claude.Client, prompt, claudeModel, effort, conversationID string) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := c.Writer.(http.Flusher)

	msgID := genID("msg_")

	// message_start
	writeSSE(c.Writer, models.AnthropicStreamMessageStart{
		Type: "message_start",
		Message: models.AnthropicStartMsg{
			ID:      msgID,
			Type:    "message",
			Role:    "assistant",
			Content: []models.AnthropicContentBlock{},
			Model:   claudeModel,
			Usage:   models.AnthropicUsage{},
		},
	})

	// content_block_start
	writeSSE(c.Writer, models.AnthropicStreamContentBlockStart{
		Type:         "content_block_start",
		Index:        0,
		ContentBlock: models.AnthropicContentBlock{Type: "text", Text: ""},
	})

	var outputChars int
	_, err := h.runCompletion(c.Request.Context(), client, prompt, claudeModel, effort, conversationID, func(text string) {
		outputChars += len(text)
		writeSSE(c.Writer, models.AnthropicStreamContentBlockDelta{
			Type:  "content_block_delta",
			Index: 0,
			Delta: models.AnthropicStreamDelta{Type: "text_delta", Text: text},
		})
		if flusher != nil {
			flusher.Flush()
		}
	})

	// content_block_stop
	writeSSE(c.Writer, models.AnthropicStreamContentBlockStop{Type: "content_block_stop", Index: 0})

	stopReason := "end_turn"
	if err != nil {
		stopReason = "error"
	}
	// message_delta
	writeSSE(c.Writer, models.AnthropicStreamMessageDelta{
		Type:  "message_delta",
		Delta: models.AnthropicStopDelta{StopReason: stopReason},
		Usage: models.AnthropicUsage{OutputTokens: outputChars / 4},
	})

	// message_stop
	writeSSE(c.Writer, models.AnthropicStreamMessageStop{Type: "message_stop"})
	if flusher != nil {
		flusher.Flush()
	}
}
