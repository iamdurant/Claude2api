package claude

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	httpclient "github.com/bogdanfinn/fhttp"
)

func TestParseRetryAfterSeconds(t *testing.T) {
	d, ok := parseRetryAfter("12", time.Unix(1000, 0))
	if !ok || d != 12*time.Second {
		t.Fatalf("got %v, %v", d, ok)
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	now := time.Unix(1000, 0)
	d, ok := parseRetryAfter(now.Add(25*time.Second).UTC().Format(http.TimeFormat), now)
	if !ok || d != 25*time.Second {
		t.Fatalf("got %v, %v", d, ok)
	}
}

func TestParseRetryAfterRejectsInvalidValue(t *testing.T) {
	if _, ok := parseRetryAfter("later", time.Unix(1000, 0)); ok {
		t.Fatal("invalid Retry-After value was accepted")
	}
}

func TestHTTPErrorFromResponsePreservesRateLimitMetadata(t *testing.T) {
	resp := &httpclient.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: httpclient.Header{
			"Retry-After": []string{"12"},
		},
		Body: httpclient.NoBody,
	}
	err := newHTTPError("send message", resp, []byte("slow down"))

	var upstreamErr *HTTPError
	if !errors.As(err, &upstreamErr) {
		t.Fatalf("error type = %T, want *HTTPError", err)
	}
	if upstreamErr.StatusCode != http.StatusTooManyRequests || upstreamErr.RetryAfter != 12*time.Second || !upstreamErr.RetryAfterSet {
		t.Fatalf("unexpected status metadata: %#v", upstreamErr)
	}
	if !strings.Contains(upstreamErr.Error(), "send message") || !strings.Contains(upstreamErr.Error(), "429") {
		t.Fatalf("unexpected error text: %s", upstreamErr.Error())
	}
}
