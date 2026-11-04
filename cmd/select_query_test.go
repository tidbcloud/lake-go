package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	dc "github.com/tidbcloud/lake-go"
)

func TestReportErrorLogsMetadataOnly(t *testing.T) {
	previous := slog.Default()
	defer slog.SetDefault(previous)
	for name, err := range map[string]error{
		"http":       dc.NewAPIError("check arguments", 400, []byte(`{"message":"diagnostic-canary"}`)),
		"query":      fmt.Errorf("wrapped: %w", &dc.QueryError{Code: 1001, Message: "diagnostic-canary", Detail: "SQL diagnostic-canary"}),
		"connection": fmt.Errorf("invalid DSN: lake://user:diagnostic-canary@host"),
	} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
			reportError("query failed", err)
			require.Contains(t, output.String(), "query failed")
			require.Contains(t, output.String(), "error_type")
			require.NotContains(t, output.String(), "diagnostic-canary")
			if name == "http" {
				require.Contains(t, output.String(), "status_code=400")
			}
			if name == "query" {
				require.Contains(t, output.String(), "query_error_code=1001")
			}
		})
	}
}
