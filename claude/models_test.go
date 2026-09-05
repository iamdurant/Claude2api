package claude

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

// TestListModelsHARReplay verifies a local capture without sending credentials.
func TestListModelsHARReplay(t *testing.T) {
	path := os.Getenv("CLAUDE_MODELS_HAR")
	if path == "" {
		t.Skip("set CLAUDE_MODELS_HAR to replay a captured bootstrap response")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("cannot open HAR")
	}
	defer file.Close()
	var har struct {
		Log struct {
			Entries []struct {
				Request struct {
					URL string `json:"url"`
				} `json:"request"`
				Response struct {
					Status  int `json:"status"`
					Content struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"response"`
			} `json:"entries"`
		} `json:"log"`
	}
	if err := json.NewDecoder(file).Decode(&har); err != nil {
		t.Fatal("cannot decode HAR")
	}
	for _, entry := range har.Log.Entries {
		u, err := url.Parse(entry.Request.URL)
		if err != nil || u.Host != "claude.ai" || !strings.HasPrefix(u.Path, "/edge-api/bootstrap/") || !strings.HasSuffix(u.Path, "/app_start") || entry.Response.Status != 200 {
			continue
		}
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" || r.URL.Path != u.Path || r.URL.Query().Encode() != u.Query().Encode() {
				t.Error("request does not match captured bootstrap endpoint and query")
				w.WriteHeader(404)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, entry.Response.Content.Text)
		}))
		defer upstream.Close()
		client, err := NewClient(upstream.URL, "test-account")
		if err != nil {
			t.Fatal(err)
		}
		client.orgID = strings.TrimSuffix(strings.TrimPrefix(u.Path, "/edge-api/bootstrap/"), "/app_start")
		ids, err := client.ListModels(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("captured bootstrap returned %d model IDs: %v", len(ids), ids)
		return
	}
	t.Fatal("HAR has no successful claude.ai app_start request")
}
