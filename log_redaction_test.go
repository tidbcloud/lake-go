package golake

import (
	"fmt"
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
