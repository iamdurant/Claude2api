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
	cfg *config.Config
}

// NewHandler creates a handler
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{cfg: cfg}
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

// newClient builds a claude.ai client from the session key in context
func (h *Handler) newClient(c *gin.Context) (*claude.Client, error) {
	sessionKey, _ := c.Get("sessionKey")
	claudeCookie, _ := c.Get("claudeCookie")
	return claude.NewClient(h.cfg.ClaudeBaseURL, sessionKey.(string), claudeCookie.(string))
}

// runCompletion drives a full claude.ai round-trip: create conversation, send
// prompt, then invoke onText for each incremental text delta.
// Returns the full accumulated text or an error.
func (h *Handler) runCompletion(ctx context.Context, client *claude.Client, prompt, claudeModel, effort string,
	onText func(text string)) (string, error) {

	convID, err := client.CreateConversation(ctx, "chat")
	if err != nil {
		return "", fmt.Errorf("create conversation: %w", err)
	}
	defer func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = client.DeleteConversation(bgCtx, convID)
		cancel()
	}()

	// Build the real request body matching claude.ai's format
	req := &models.ClaudeCompletionRequest{
		Prompt:            prompt,
		ParentMessageUUID: utils.GenerateUUID(),
		Timezone:          h.cfg.Timezone,
		Locale:            h.cfg.Locale,
		Model:             claudeModel,
		Effort:            effort,
		ThinkingMode:      "auto",
		TurnMessageUUIDs: &models.TurnMessageUUIDs{
			HumanMessageUUID:     utils.GenerateUUID(),
			AssistantMessageUUID: utils.GenerateUUID(),
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
		return "", fmt.Errorf("send message: %w", err)
	}

	var sb strings.Builder
	for evt := range events {
		if evt.Error != nil {
			return sb.String(), fmt.Errorf("upstream: %s", evt.Error.Message)
		}
		text := claude.ExtractTextFromSSE(evt)
		if text != "" {
			sb.WriteString(text)
			if onText != nil {
				onText(text)
			}
		}
		if claude.IsStopEvent(evt) {
			break
		}
	}
	return sb.String(), nil
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
