package claude

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	httpclient "github.com/bogdanfinn/fhttp"
)

// HTTPError preserves an upstream HTTP status through the client call stack.
type HTTPError struct {
	Operation     string
	StatusCode    int
	Message       string
	RetryAfter    time.Duration
	RetryAfterSet bool
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: status %d", e.Operation, e.StatusCode)
}

func IsStatus(err error, statusCode int) bool {
	var upstreamErr *HTTPError
	return errors.As(err, &upstreamErr) && upstreamErr.StatusCode == statusCode
}

func RetryAfterOf(err error) (time.Duration, bool) {
	var upstreamErr *HTTPError
	if !errors.As(err, &upstreamErr) || !upstreamErr.RetryAfterSet {
		return 0, false
	}
	return upstreamErr.RetryAfter, true
}

func newHTTPError(operation string, resp *httpclient.Response, body []byte) error {
	retryAfter, retryAfterSet := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	message := strings.TrimSpace(string(body))
	if len(message) > 8*1024 {
		message = message[:8*1024]
	}
	return &HTTPError{
		Operation:     operation,
		StatusCode:    resp.StatusCode,
		Message:       message,
		RetryAfter:    retryAfter,
		RetryAfterSet: retryAfterSet,
	}
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	if when.Before(now) {
		return 0, true
	}
	return when.Sub(now), true
}
