package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"claude2api/models"
	"claude2api/utils"

	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// Client is the reverse-engineered claude.ai API client with TLS fingerprint bypass
type Client struct {
	httpClient   tlsclient.HttpClient
	baseURL      string
	sessionKey   string
	claudeCookie string
	orgID        string // cached org UUID
	deviceID     string // anthropic-device-id
}

// NewClient creates a new claude.ai API client using Chrome 146 TLS fingerprint
func NewClient(baseURL, sessionKey string, claudeCookie ...string) (*Client, error) {
	cookie := ""
	if len(claudeCookie) > 0 {
		cookie = claudeCookie[0]
	}
	deviceID := cookieValue(cookie, "anthropic-device-id")
	if deviceID == "" {
		deviceID = utils.GenerateUUID()
	}
	jar := tlsclient.NewCookieJar()
	options := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(300),
		tlsclient.WithClientProfile(profiles.Chrome_146),
		tlsclient.WithCookieJar(jar),
		tlsclient.WithNotFollowRedirects(),
	}

	httpClient, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("create tls-client: %w", err)
	}

	return &Client{
		httpClient:   httpClient,
		baseURL:      strings.TrimRight(baseURL, "/"),
		sessionKey:   sessionKey,
		claudeCookie: cookie,
		orgID:        cookieValue(cookie, "lastActiveOrg"),
		deviceID:     deviceID,
	}, nil
}

func cookieValue(cookie, name string) string {
	for _, part := range strings.Split(cookie, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 && kv[0] == name {
			return kv[1]
		}
	}
	return ""
}

// doRequest performs an authenticated HTTP request to claude.ai
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path

	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, body)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	// === Headers from real Chrome 146 on claude.ai (captured via js-reverse) ===
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36")
	req.Header.Set("Origin", c.baseURL)
	req.Header.Set("Referer", c.baseURL+"/")
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-origin")
	// Claude.ai specific headers
	req.Header.Set("anthropic-client-platform", "web_claude_ai")
	req.Header.Set("anthropic-device-id", c.deviceID)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Authentication: prefer the full browser Cookie header when available.
	if c.claudeCookie != "" {
		req.Header.Set("Cookie", c.claudeCookie)
	} else {
		req.AddCookie(&http.Cookie{Name: "sessionKey", Value: c.sessionKey})
		req.AddCookie(&http.Cookie{Name: "sessionKeyLC", Value: "1"})
		req.AddCookie(&http.Cookie{Name: "anthropic-device-id", Value: c.deviceID})
		if c.orgID != "" {
			req.AddCookie(&http.Cookie{Name: "lastActiveOrg", Value: c.orgID})
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http do: %w", err)
	}

	return resp, nil
}

// GetOrganization fetches the user's organization UUID
func (c *Client) GetOrganization(ctx context.Context) (string, error) {
	if c.orgID != "" {
		return c.orgID, nil
	}

	resp, err := c.doRequest(ctx, "GET", "/api/organizations", nil)
	if err != nil {
		return "", fmt.Errorf("get organizations: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get organizations: status %d: %s", resp.StatusCode, string(body))
	}

	var orgs models.ClaudeOrganizationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&orgs); err != nil {
		return "", fmt.Errorf("decode organizations: %w", err)
	}

	if len(orgs) == 0 {
		return "", fmt.Errorf("no organizations found")
	}

	c.orgID = orgs[0].UUID
	return c.orgID, nil
}

// CreateConversation creates a new conversation and returns its UUID
func (c *Client) CreateConversation(ctx context.Context, title string) (string, error) {
	orgID, err := c.GetOrganization(ctx)
	if err != nil {
		return "", err
	}

	reqBody := models.ClaudeCreateConversationRequest{
		Name:             title,
		OrganizationUUID: orgID,
	}
	body, _ := utils.JSONToReader(reqBody)

	resp, err := c.doRequest(ctx, "POST",
		fmt.Sprintf("/api/organizations/%s/chat_conversations", orgID), body)
	if err != nil {
		return "", fmt.Errorf("create conversation: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create conversation: status %d: %s", resp.StatusCode, string(respBody))
	}

	var conv models.ClaudeConversation
	if err := json.NewDecoder(resp.Body).Decode(&conv); err != nil {
		return "", fmt.Errorf("decode conversation: %w", err)
	}

	return conv.UUID, nil
}

// DeleteConversation deletes a conversation by UUID
func (c *Client) DeleteConversation(ctx context.Context, convID string) error {
	orgID, err := c.GetOrganization(ctx)
	if err != nil {
		return err
	}

	resp, err := c.doRequest(ctx, "DELETE",
		fmt.Sprintf("/api/organizations/%s/chat_conversations/%s", orgID, convID), nil)
	if err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete conversation: status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SendMessage sends a message and returns a channel of SSE completion events.
func (c *Client) SendMessage(ctx context.Context, convID string, req *models.ClaudeCompletionRequest) (<-chan models.ClaudeCompletionEvent, error) {
	orgID, err := c.GetOrganization(ctx)
	if err != nil {
		return nil, err
	}

	body, _ := utils.JSONToReader(req)

	path := fmt.Sprintf("/api/organizations/%s/chat_conversations/%s/completion",
		orgID, convID)

	resp, err := c.doRequest(ctx, "POST", path, body)
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("send message: status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse SSE stream in background
	events := make(chan models.ClaudeCompletionEvent, 64)
	go func() {
		defer close(events)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)

		var eventType, data string
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()

			if line == "" {
				if data != "" {
					events <- parseCompletionEvent(eventType, data)
				}
				eventType = ""
				data = ""
				continue
			}

			if strings.HasPrefix(line, "event: ") {
				eventType = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				payload := strings.TrimPrefix(line, "data: ")
				if data != "" {
					data += "\n"
				}
				data += payload
			}
		}

		if data != "" {
			events <- parseCompletionEvent(eventType, data)
		}
	}()

	return events, nil
}

func parseCompletionEvent(eventType, data string) models.ClaudeCompletionEvent {
	var evt models.ClaudeCompletionEvent
	evt.Type = eventType
	evt.Data = data
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return evt
	}
	if len(evt.Delta) > 0 {
		switch evt.Type {
		case "content_block_delta":
			var delta models.ClaudeCompletionDelta
			if err := json.Unmarshal(evt.Delta, &delta); err == nil {
				evt.TextDelta = &delta
			}
		case "message_delta":
			var delta models.MessageDeltaPayload
			if err := json.Unmarshal(evt.Delta, &delta); err == nil {
				evt.MessageDelta = &delta
			}
		}
	}
	return evt
}

// BuildPrompt converts OpenAI messages into a single prompt string for claude.ai
func BuildPrompt(messages []models.Message) string {
	var parts []string
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			parts = append(parts, "[System]\n"+msg.Content)
		case "user":
			parts = append(parts, "[Human]\n"+msg.Content)
		case "assistant":
			parts = append(parts, "[Assistant]\n"+msg.Content)
		default:
			parts = append(parts, msg.Content)
		}
	}
	parts = append(parts, "[Assistant]\n")
	return strings.Join(parts, "\n\n")
}

// ExtractTextFromSSE extracts text from a claude.ai SSE event
func ExtractTextFromSSE(evt models.ClaudeCompletionEvent) string {
	// content_block_delta with text_delta
	if evt.TextDelta != nil && evt.TextDelta.Text != "" {
		return evt.TextDelta.Text
	}
	return ""
}

// IsStopEvent returns true if the event signals end of stream
func IsStopEvent(evt models.ClaudeCompletionEvent) bool {
	return evt.Type == "message_stop" || evt.Type == "message_delta" && evt.MessageDelta != nil && evt.MessageDelta.StopReason != ""
}
