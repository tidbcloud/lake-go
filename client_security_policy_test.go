package golake

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegisteredTLSConfigAuthenticatesServer(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := testHTTPConfig(t, server.URL)
	cfg.SSLMode = "require"
	cfg.TLSConfig = "test-lake-tls"
	cfg.EnableOpenTelemetry = true
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	require.NoError(t, RegisterTLSConfig(cfg.TLSConfig, &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}))
	t.Cleanup(func() { DeregisterTLSConfig(cfg.TLSConfig) })

	response, err := NewAPIHttpClientFromConfig(cfg).Get(server.URL)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusNoContent, response.StatusCode)

	DeregisterTLSConfig(cfg.TLSConfig)
	_, err = NewAPIHttpClientFromConfig(cfg).Get(server.URL)
	require.ErrorContains(t, err, "unknown Lake TLS configuration")
}

func TestStateRejectsTamperingAndCrossIdentityRestore(t *testing.T) {
	cfg := NewConfig()
	cfg.Host = "warehouse.example:443"
	cfg.User = "alice"
	cfg.Role = "reader"
	original := NewAPIClientFromConfig(cfg)
	snapshot := original.GetState()
	wire, err := json.Marshal(snapshot)
	require.NoError(t, err)
	var restoredState APIClientState
	require.NoError(t, json.Unmarshal(wire, &restoredState))
	require.NoError(t, NewAPIClientFromConfig(cfg).RestoreState(&restoredState))

	other := *cfg
	other.Host = "other.example:443"
	require.Error(t, NewAPIClientFromConfig(&other).RestoreState(snapshot))
	other = *cfg
	other.Role = "admin"
	require.Error(t, NewAPIClientFromConfig(&other).RestoreState(snapshot))

	restoredState.SessionState = `{"role":"admin"}`
	client := NewAPIClientFromConfig(cfg)
	initialID := client.SessionID
	require.Error(t, client.RestoreState(&restoredState))
	require.Equal(t, initialID, client.SessionID)
	require.False(t, client.stateRestored)
	require.Error(t, client.WithState(&restoredState).initializeConnectionInfo(context.Background()))
}

func TestStateIsBoundToBearerCredential(t *testing.T) {
	cfg := NewConfig()
	cfg.AccessToken = "first-test-token"
	state := NewAPIClientFromConfig(cfg).GetState()
	require.NotEmpty(t, state.Signature)
	cfg.AccessToken = "second-test-token"
	require.Error(t, NewAPIClientFromConfig(cfg).RestoreState(state))
}
