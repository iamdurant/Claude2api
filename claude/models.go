package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// ListModels reads the model catalog from the web app bootstrap response.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	orgID, err := c.GetOrganization(ctx)
	if err != nil {
		return nil, fmt.Errorf("list web models: organization lookup failed")
	}
	path := "/edge-api/bootstrap/" + url.PathEscape(orgID) + "/app_start?statsig_hashing_algorithm=djb2&growthbook_format=sdk&cache_bust=1&include_system_prompts=false"
	resp, err := c.doRequest(ctx, "GET", path, nil, "/new")
	if err != nil {
		return nil, fmt.Errorf("list web models: request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("list web models: status %d", resp.StatusCode)
	}

	var bootstrap struct {
		AvailableModels struct {
			Models []struct {
				ID string `json:"model_id"`
			} `json:"models"`
		} `json:"claude_ai_available_models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&bootstrap); err != nil {
		return nil, fmt.Errorf("list web models: invalid bootstrap response")
	}

	ids := make([]string, 0, len(bootstrap.AvailableModels.Models))
	seen := make(map[string]bool)
	for _, model := range bootstrap.AvailableModels.Models {
		id := strings.TrimSpace(model.ID)
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("list web models: bootstrap returned no model IDs")
	}
	return ids, nil
}
