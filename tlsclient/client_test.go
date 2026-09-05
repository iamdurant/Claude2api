package tlsclient

import (
	"context"
	"strings"
	"testing"
)

func TestChromeHeaders(t *testing.T) {
	client, err := New("https://claude.ai/")
	if err != nil {
		t.Fatal(err)
	}
	req, err := client.NewRequest(context.Background(), "GET", "/api/organizations", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("User-Agent"); !strings.Contains(got, "Chrome/152.0.0.0") {
		t.Errorf("unexpected User-Agent: %s", got)
	}
	want := `"Google Chrome";v="152", "Not.A/Brand";v="8", "Chromium";v="152"`
	if got := req.Header.Get("sec-ch-ua"); got != want {
		t.Errorf("sec-ch-ua = %q, want %q", got, want)
	}
	if got := req.Header.Get("sec-ch-ua-platform"); got != `"Windows"` {
		t.Errorf("unexpected platform: %s", got)
	}
	if got := req.Header.Get("sec-ch-ua-mobile"); got != "?0" {
		t.Errorf("unexpected mobile: %s", got)
	}
	if req.URL.String() != "https://claude.ai/api/organizations" {
		t.Errorf("unexpected URL: %s", req.URL)
	}
}
