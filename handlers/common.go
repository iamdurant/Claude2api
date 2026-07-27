package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"claude2api/claude"
	"claude2api/config"
	"claude2api/models"
	"claude2api/utils"

	"github.com/gin-gonic/gin"
)

// Handler holds shared config and routes requests to claude.ai
type Handler struct {
	cfg           *config.Config
	conversations *conversationStore
}

// NewHandler creates a handler
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{cfg: cfg, conversations: newConversationStore()}
}

// resolveModel returns the claude.ai model id for a requested model, or an error
func resolveModel(requested, fallback string) (string, error) {
	if requested == "" {
		requested = fallback
	}
	m, ok := config.SupportedModels[requested]
	if !ok {
		return "", fmt.Errorf("model '%s' is not supported", requested)
	}
	return m, nil
}

// resolveEffort validates an effort string and returns a claude.ai-safe value.
func resolveEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low", "medium", "high", "xhigh", "max":
		return strings.ToLower(strings.TrimSpace(effort))
	default:
		return "medium"
	}
}

// resolveThinking maps Claude Code's thinking config to claude.ai's thinking_mode.
// Claude Code sends {"type":"enabled","budget_tokens":N} or {"type":"disabled"}.
// claude.ai accepts "auto" | "none". We also return the budget for potential
// future use (claude.ai web does not expose a budget knob).
func resolveThinking(thinking interface{}) (thinkingMode string, budgetTokens int) {
	if thinking == nil {
		return "auto", 0
	}
	m, ok := thinking.(map[string]interface{})
	if !ok {
		return "auto", 0
	}
	typ, _ := m["type"].(string)
	switch typ {
	case "disabled":
		return "none", 0
	case "enabled":
		budget, _ := m["budget_tokens"].(float64)
		return "auto", int(budget)
	default:
		return "auto", 0
	}
}

// newClient builds a claude.ai client from the session key in context
func (h *Handler) newClient(c *gin.Context) (*claude.Client, error) {
	sessionKey, _ := c.Get("sessionKey")
	claudeCookie, _ := c.Get("claudeCookie")
	return claude.NewClient(h.cfg.ClaudeBaseURL, sessionKey.(string), claudeCookie.(string))
}

// runCompletion drives a full claude.ai round-trip: create conversation, send
// prompt, then invoke onText for each incremental text delta.
// Returns the accumulated thinking text, the accumulated text, or an error.
// When tools is nil, the default claude.ai web tools are sent; pass an empty
// slice (or custom payload) to override — used by the tool-simulation loop.
// thinking overrides the proxy-level thinking config (e.g. from request body).
func (h *Handler) runCompletion(ctx context.Context, client *claude.Client, prompt, claudeModel, effort, conversationID string,
	onText func(text string), tools []json.RawMessage, thinkingOverride ...string) (string, string, error) {

	convID := ""
	persistent := conversationID != ""
	var state *conversationState
	if persistent {
		if existing, ok := h.conversations.get(conversationID); ok {
			state = existing
			convID = existing.ClaudeConversationID
		} else {
			createdID, err := client.CreateConversation(ctx, "chat")
			if err != nil {
				return "", "", fmt.Errorf("create conversation: %w", err)
			}
			convID = createdID
			state = &conversationState{ClientConversationID: conversationID, ClaudeConversationID: convID}
			h.conversations.set(state)
		}
	} else {
		createdID, err := client.CreateConversation(ctx, "chat")
		if err != nil {
			return "", "", fmt.Errorf("create conversation: %w", err)
		}
		convID = createdID
		defer func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = client.DeleteConversation(bgCtx, convID)
			cancel()
		}()
	}

	humanUUID := utils.GenerateUUID()
	assistantUUID := utils.GenerateUUID()
	parentUUID := utils.GenerateUUID()
	if state != nil && state.LastAssistantUUID != "" {
		parentUUID = state.LastAssistantUUID
	}

	// Thinking: per-request override (from Claude Code's request body) wins,
	// else fall back to proxy-level config.
	thinkingMode, _ := resolveThinking(h.cfg.Thinking)
	if len(thinkingOverride) > 0 && thinkingOverride[0] != "" {
		thinkingMode = thinkingOverride[0]
	}

	// Choose tools payload: caller override, or default web tools.
	toolsPayload := claude.WebTools()
	if len(tools) > 0 {
		toolsPayload = tools[0]
	}

	// Build the real request body matching claude.ai's format
	req := &models.ClaudeCompletionRequest{
		Prompt:            prompt,
		ParentMessageUUID: parentUUID,
		Timezone:          h.cfg.Timezone,
		Locale:            h.cfg.Locale,
		Model:             claudeModel,
		Effort:            effort,
		ThinkingMode:      thinkingMode,
		Tools:             toolsPayload,
		TurnMessageUUIDs: &models.TurnMessageUUIDs{
			HumanMessageUUID:     humanUUID,
			AssistantMessageUUID: assistantUUID,
		},
		Attachments:   []models.ClaudeAttachment{},
		Files:         []models.ClaudeFile{},
		SyncSources:   []interface{}{},
		RenderingMode: "messages",
		CreateConversationParams: &models.CreateConversationParams{
			Name:                           "",
			Model:                          claudeModel,
			IncludeConversationPreferences: true,
			PaprikaMode:                    nil,
			CompassMode:                    nil,
			ToolSearchMode:                 "auto",
			IsTemporary:                    false,
			EnabledImagine:                 true,
		},
	}

	events, err := client.SendMessage(ctx, convID, req)
	if err != nil {
		return "", "", fmt.Errorf("send message: %w", err)
	}

	var thinkingBuf, sb strings.Builder
	for evt := range events {
		if evt.Error != nil {
			return thinkingBuf.String(), sb.String(), fmt.Errorf("upstream: %s", evt.Error.Message)
		}
		// Capture thinking blocks.
		if t := claude.ExtractThinkingFromSSE(evt); t != "" {
			thinkingBuf.WriteString(t)
		}
		// Capture text blocks.
		if text := claude.ExtractTextFromSSE(evt); text != "" {
			sb.WriteString(text)
			if onText != nil {
				onText(text)
			}
		}
		if claude.IsStopEvent(evt) {
			break
		}
	}
	if persistent {
		h.conversations.touch(conversationID, humanUUID, assistantUUID)
	}
	return thinkingBuf.String(), sb.String(), nil
}

// writeSSE writes a "data: {...}\n\n" SSE line
func writeSSE(w io.Writer, payload interface{}) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = io.WriteString(w, "data: "+string(b)+"\n\n")
}

// ListModels returns OpenAI-compatible /v1/models
func (h *Handler) ListModels(c *gin.Context) {
	data := make([]models.ModelInfo, 0, len(config.SupportedModels))
	now := time.Now().Unix()
	for id := range config.SupportedModels {
		data = append(data, models.ModelInfo{
			ID:      id,
			Object:  "model",
			Created: now,
			OwnedBy: "anthropic",
		})
	}
	c.JSON(http.StatusOK, models.ModelsResponse{Object: "list", Data: data})
}

// genID returns a short id like "chatcmpl-xxxxxxxx"
func genID(prefix string) string {
	return prefix + utils.GenerateUUID()[:8]
}

// error helpers
func badRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": msg, "type": "invalid_request_error"}})
}

func internalError(c *gin.Context, msg string) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": msg, "type": "internal_error"}})
}

func upstreamError(c *gin.Context, msg string) {
	c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": msg, "type": "upstream_error"}})
}
