package golake

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeURLKeepsRequestOnConfiguredEndpoint(t *testing.T) {
	c := &APIClient{apiEndpoint: "https://lake.example.com", host: "lake.example.com"}

	ok := map[string]string{
		"/v1/query":                       "https://lake.example.com/v1/query",
		"/v1/query/page/1":                "https://lake.example.com/v1/query/page/1",
		"/v1/session/x/final?limit=1":     "https://lake.example.com/v1/session/x/final?limit=1",
		"https://lake.example.com/v1/q":   "https://lake.example.com/v1/q",
		"https://lake.example.com/v1/%2e": "https://lake.example.com/v1/%2e",
	}
	for path, want := range ok {
		got, err := c.makeURL(path)
		require.NoError(t, err, path)
		assert.Equal(t, want, got, path)
	}

	// A response can name any host it likes; the request must not follow it.
	bad := []string{
		"@attacker.example/v1/query",
		"https://lake.example.com@attacker.example/v1/query",
		"//attacker.example/v1/query",
		"https://attacker.example/v1/query",
		"http://lake.example.com/v1/query", // scheme downgrade
		"attacker.example/v1/query",        // relative path that is not rooted
	}
	for _, path := range bad {
		got, err := c.makeURL(path)
		assert.Error(t, err, path)
		assert.Empty(t, got, path)
	}
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
