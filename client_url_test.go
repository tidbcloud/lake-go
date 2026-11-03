package golake

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeURL(t *testing.T) {
	c := &APIClient{apiEndpoint: "https://lake.example.com", host: "lake.example.com"}

	ok := []struct {
		name string
		path string
		want string
	}{
		{"fixed path", "/v1/query", "https://lake.example.com/v1/query"},
		{"page path with query", "/v1/query/abc/page/2?limit=1", "https://lake.example.com/v1/query/abc/page/2?limit=1"},
		{"escaped path", "/v1/query/abc%2Fdef/page/2", "https://lake.example.com/v1/query/abc%2Fdef/page/2"},
		{"empty next_uri (endpoint itself)", "", "https://lake.example.com"},
		{"query-only next_uri", "?page=2", "https://lake.example.com?page=2"},
		{"same-host absolute URL", "https://lake.example.com/v1/query/final", "https://lake.example.com/v1/query/final"},
		{"same-host absolute URL, case-insensitive host", "https://LAKE.Example.COM/v1/query", "https://LAKE.Example.COM/v1/query"},
	}
	for _, tc := range ok {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.makeURL(tc.path)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	// A response can name any host it likes; the request must not follow it.
	bad := []struct {
		name    string
		path    string
		wantMsg string
	}{
		{"other host absolute URL", "https://other.example/v1/query", "refusing to send a request to"},
		{"userinfo in absolute URL", "https://lake.example.com@attacker.example/v1/query", "refusing to send a request to"},
		{"protocol-relative URL", "//attacker.example/v1/query", "refusing to send a request to"},
		{"scheme downgrade", "http://lake.example.com/v1/query", "refusing to send a request to"},
		{"userinfo form", "@attacker.example/v1/query", "invalid request path"},
		{"non-rooted relative path", "attacker.example/v1/query", "invalid request path"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.makeURL(tc.path)
			require.Error(t, err)
			assert.Empty(t, got)
			assert.Contains(t, err.Error(), tc.wantMsg)
			// Callers still classify these as request failures, and a wrapping
			// error in a higher layer must not hide that.
			assert.True(t, errors.Is(err, ErrDoRequest), "errors.Is(err, ErrDoRequest)")
			assert.True(t, errors.Is(fmt.Errorf("failed to query page: %w", err), ErrDoRequest),
				"errors.Is must survive a wrapping error")
		})
	}
}

func TestMakeURLInvalidPathIsErrDoRequest(t *testing.T) {
	c := &APIClient{apiEndpoint: "https://lake.example.com", host: "lake.example.com"}
	_, err := c.makeURL("v1/query")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid request path")
	assert.True(t, errors.Is(err, ErrDoRequest))
}

// TestServerSuppliedURIisNotFollowed is the end-to-end version of the check
// above: even if a hostile response names another host, doRequest must fail
// locally instead of sending the credentials there.
func TestServerSuppliedURIIsNotFollowed(t *testing.T) {
	var hits int32
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()

	c := &APIClient{
		apiEndpoint: "http://lake.example.invalid",
		host:        "lake.example.invalid",
		user:        "warehouse_user",
		password:    "warehouse_password",
		cli:         http.DefaultClient,
	}

	err := c.doRequest(context.Background(), "GET", attacker.URL+"/v1/query/page/1", nil, false, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to send a request to")

	assert.Equal(t, int32(0), atomic.LoadInt32(&hits), "request must not reach the host named by the response")
}

// TestNextURICannotRedirectPolling covers the actual source of the value: the
// next_uri/final_uri/kill_uri fields of a QueryResponse.
func TestNextURICannotRedirectPolling(t *testing.T) {
	var hits int32
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()

	c := &APIClient{
		apiEndpoint:               "http://lake.example.invalid",
		host:                      "lake.example.invalid",
		user:                      "warehouse_user",
		password:                  "warehouse_password",
		cli:                       http.DefaultClient,
		connectionInfoInitialized: true,
	}

	// "@host/path" is how a bare concatenation with a trusted endpoint would
	// have been turned into userinfo + attacker host.
	_, err := c.PollQuery(context.Background(), "@"+strings.TrimPrefix(attacker.URL, "http://")+"/v1/query/page/1")
	require.Error(t, err)
	assert.Equal(t, int32(0), atomic.LoadInt32(&hits))
}
