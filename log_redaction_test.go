package golake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A client used to be logged as a whole struct on every pooled-connection
// teardown when debug=1, which wrote the warehouse password to stderr. Formatting
// a client must never render the credential.
func TestAPIClientStringRedactsCredentials(t *testing.T) {
	c := &APIClient{
		SessionID: "session-1",
		host:      "lake.example.com",
		tenant:    "tn1",
		warehouse: "small-abc",
		user:      "warehouse_user",
		password:  "s3cr3t-password",
	}

	for _, format := range []string{"%v", "%+v", "%s"} {
		got := fmt.Sprintf(format, c)
		assert.NotContains(t, got, "s3cr3t-password", format)
		assert.Contains(t, got, "<redacted>", format)
	}
	assert.Contains(t, fmt.Sprintf("%v", c), "session-1")
}

// A Config carries both the password and the bearer token. It is a common
// mistake to log it when a DSN cannot be built, so formatting it is redacted too.
func TestConfigStringRedactsCredentials(t *testing.T) {
	cfg := NewConfig()
	cfg.User = "warehouse_user"
	cfg.Password = "s3cr3t-password"
	cfg.AccessToken = "s3cr3t-token"

	for _, format := range []string{"%v", "%+v"} {
		got := fmt.Sprintf(format, cfg)
		assert.NotContains(t, got, "s3cr3t-password", format)
		assert.NotContains(t, got, "s3cr3t-token", format)
		assert.Contains(t, got, "<redacted>", format)
	}

	// FormatDSN is the explicit, documented way to get the credential-bearing
	// form; it must keep working unchanged.
	assert.Contains(t, cfg.FormatDSN(), "s3cr3t-password")
	assert.Contains(t, cfg.FormatDSN(), "access_token=s3cr3t-token")
}

func TestRedacted(t *testing.T) {
	assert.Equal(t, "", redacted(""))
	assert.Equal(t, "<redacted>", redacted("hunter2"))
}

func TestStartQueryLogsMetadataWithoutSQL(t *testing.T) {
	const sql = "COPY INTO t FROM 's3://bucket' CONNECTION=(SECRET_ACCESS_KEY='log-canary')"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request QueryRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, sql, request.SQL)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"test-query","data":[],"state":"Succeeded"}`))
	}))
	defer server.Close()
	var output bytes.Buffer
	testLogger := CreateDefaultLogger()
	require.NoError(t, testLogger.SetLogLevel("debug"))
	testLogger.SetOutput(&output)
	previousLogger := GetLogger()
	SetLogger(testLogger)
	defer SetLogger(previousLogger)
	client := &APIClient{apiEndpoint: server.URL, cli: server.Client(), user: "test-user", password: "test-password"}
	_, err := client.startQuery(context.Background(), sql, queryTransportJSON)
	require.NoError(t, err)
	assert.Contains(t, output.String(), "start query")
	assert.NotContains(t, output.String(), "log-canary")
	assert.NotContains(t, output.String(), sql)
}
