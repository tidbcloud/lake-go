package golake

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIErrorDiagnosticsDoNotRenderResponse(t *testing.T) {
	const secret = "response-credential-canary"
	for name, body := range map[string]string{
		"json":       `{"error":"response-credential-canary","message":"SQL contains response-credential-canary"}`,
		"proxy text": "proxy rejected token=" + secret,
	} {
		t.Run(name, func(t *testing.T) {
			err := NewAPIError("please check your arguments", 400, []byte(body))
			for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
				rendered := fmt.Sprintf(format, err)
				require.NotContains(t, rendered, secret)
				require.Contains(t, rendered, "400")
				require.Contains(t, rendered, "please check your arguments")
			}
			var output bytes.Buffer
			slog.New(slog.NewJSONHandler(&output, nil)).Error("request failed", "error", err)
			require.NotContains(t, output.String(), secret)
			require.Contains(t, output.String(), "status_code")
			// Programmatic access is unchanged and remains an explicit opt-in.
			var apiErr APIError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, body, apiErr.RespText)
			require.Equal(t, apiErr.RespBody, RespBody(fmt.Errorf("wrapped: %w", err)))
		})
	}
}

func TestAPIErrorClassificationSurvivesSafeFormatting(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", NewAPIError("authorization failed", 401, []byte("credential-canary")))
	require.True(t, IsAuthFailed(err))
	require.False(t, IsNotFound(err))
	require.True(t, IsNotFound(NewAPIError("missing", 404, nil)))
	require.True(t, IsProxyErr(NewAPIError("proxy", 520, nil)))
	// A timeout mentioned only in an unused JSON field did not trigger retries
	// before diagnostic redaction and must not start doing so now.
	require.False(t, isProvisionWarehouseTimeout(NewAPIError("internal server error", 500,
		[]byte(`{"message":"permission denied","error":"ProvisionWarehouseTimeout"}`))))
}

func TestQueryRetryReadsExplicitResponse(t *testing.T) {
	client := &APIClient{}
	attempts := 0
	err := client.doRetry(func() error {
		attempts++
		if attempts == 1 {
			return fmt.Errorf("wrapped: %w", NewAPIError("internal server error", 500,
				[]byte(`{"message":"ProvisionWarehouseTimeout: credential-canary"}`)))
		}
		return nil
	}, Query)
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
}
