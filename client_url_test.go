package golake

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPRedirectsCannotLeaveEndpoint(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, transport := range []string{"json", "arrow", "upload"} {
			for _, otherHost := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%s/other-host=%t", status, transport, otherHost), func(t *testing.T) {
					var outsideHits, credentialLeaks atomic.Int32
					outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						outsideHits.Add(1)
						if r.Header.Get("Authorization") != "" || strings.Contains(r.Header.Get("Cookie"), "session-canary") {
							credentialLeaks.Add(1)
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{}`))
					}))
					defer outside.Close()
					target := outside.URL + "/capture"
					if otherHost {
						target = strings.Replace(target, "127.0.0.1", "localhost", 1)
					}

					var endpointHits atomic.Int32
					endpoint := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						endpointHits.Add(1)
						assert.NotEmpty(t, r.Header.Get("Authorization"))
						if transport != "upload" {
							assert.Contains(t, r.Header.Get("Cookie"), "session-canary")
						}
						// The first hop stays on the endpoint, so validation must
						// also cover subsequent redirects in the chain.
						if r.URL.Path != "/redirect-hop" {
							http.Redirect(w, r, "/redirect-hop", http.StatusTemporaryRedirect)
						} else {
							http.Redirect(w, r, target, status)
						}
					}))
					if transport == "upload" {
						endpoint.Start()
					} else {
						endpoint.StartTLS()
					}
					defer endpoint.Close()
					cfg := NewConfig()
					base, err := url.Parse(endpoint.URL)
					require.NoError(t, err)
					cfg.Host = base.Host
					if base.Scheme == "http" {
						cfg.SSLMode = SSL_MODE_DISABLE
					}
					cfg.User, cfg.Password = "review-user", "password-canary"
					if transport == "arrow" {
						cfg.User, cfg.Password, cfg.AccessToken = "", "", "token-canary"
					}
					client := NewAPIClientFromConfig(cfg)
					client.cli.Transport = endpoint.Client().Transport
					client.cli.Jar.SetCookies(base, []*http.Cookie{{Name: "session", Value: "session-canary", Secure: true}})

					switch transport {
					case "json":
						err = client.doRequest(context.Background(), "GET", "/v1/query/review/page/1", nil, false, nil, nil)
					case "arrow":
						_, err = client.doRequestRaw(context.Background(), "GET", "/v1/query/review/page/1", nil, false, arrowStreamContentType)
					case "upload":
						err = client.UploadToStageByAPI(context.Background(), &StageLocation{Name: "review", Path: "data.csv"}, bufio.NewReader(strings.NewReader("1\n")))
					}
					assert.ErrorIs(t, err, ErrDoRequest)
					if assert.Error(t, err) {
						assert.Contains(t, err.Error(), "refusing to send a request to")
					}
					assert.Equal(t, int32(2), endpointHits.Load())
					assert.Zero(t, outsideHits.Load(), "the redirect target must not receive any request")
					assert.Zero(t, credentialLeaks.Load(), "credentials must stay on the configured endpoint")
				})
			}
		}
	}
}

func TestSameEndpointHTTPRedirects(t *testing.T) {
	for _, transport := range []string{"json", "arrow", "upload"} {
		t.Run(transport, func(t *testing.T) {
			var finalHits atomic.Int32
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Basic "+encode("review-user", "password-canary"), r.Header.Get("Authorization"))
				if r.URL.Path != "/final" {
					http.Redirect(w, r, "http://"+r.Host+"/final", http.StatusTemporaryRedirect)
					return
				}
				finalHits.Add(1)
				if transport == "upload" {
					assert.Equal(t, "PUT", r.Method)
					err := r.ParseMultipartForm(1024)
					assert.NoError(t, err)
					if r.MultipartForm != nil {
						defer r.MultipartForm.RemoveAll()
						assert.Len(t, r.MultipartForm.File["upload"], 1)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer endpoint.Close()
			cfg := NewConfig()
			cfg.Host = strings.TrimPrefix(endpoint.URL, "http://")
			cfg.SSLMode = SSL_MODE_DISABLE
			cfg.User, cfg.Password = "review-user", "password-canary"
			client := NewAPIClientFromConfig(cfg)
			var err error
			switch transport {
			case "json":
				err = client.doRequest(context.Background(), "GET", "/v1/query/review/page/1", nil, false, nil, nil)
			case "arrow":
				_, err = client.doRequestRaw(context.Background(), "GET", "/v1/query/review/page/1", nil, false, arrowStreamContentType)
			case "upload":
				err = client.UploadToStageByAPI(context.Background(), &StageLocation{Name: "review", Path: "data.csv"}, bufio.NewReader(strings.NewReader("1\n")))
			}
			require.NoError(t, err)
			assert.Equal(t, int32(1), finalHits.Load())
		})
	}
}

func TestHTTPRedirectLimit(t *testing.T) {
	var hits atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer endpoint.Close()
	cfg := NewConfig()
	cfg.Host = strings.TrimPrefix(endpoint.URL, "http://")
	cfg.SSLMode = SSL_MODE_DISABLE
	cfg.User, cfg.Password = "review-user", "password-canary"
	client := NewAPIClientFromConfig(cfg)
	err := client.doRequest(context.Background(), "GET", "/v1/query/review/page/1", nil, false, nil, nil)
	require.ErrorContains(t, err, "stopped after 10 redirects")
	assert.Equal(t, int32(10), hits.Load())
}

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
