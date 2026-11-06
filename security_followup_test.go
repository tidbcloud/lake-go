package golake

import (
	"bufio"
	"context"
	"database/sql/driver"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDSNAuthorityStable(t *testing.T) {
	for _, dsn := range []string{"lake://gateway.example/x@evil.example?access_token=dummy", "lake://gateway.example/default?warehouse=w@evil.example", "lake://gateway.example:443/x@y", "lake://gateway.example/#user:password@evil.example"} {
		cfg, e := ParseDSN(dsn)
		require.NoError(t, e)
		require.Equal(t, "gateway.example:443", cfg.Host)
	}
	cfg := NewConfig()
	cfg.Host = "gateway.example:443"
	cfg.AccessToken = "dummy"
	cfg.Database = "x@evil.example"
	parsed, e := ParseDSN(cfg.FormatDSN())
	require.NoError(t, e)
	require.Equal(t, cfg.Host, parsed.Host)
	require.Equal(t, cfg.Database, parsed.Database)
}
func TestNamedStringWrappersEscape(t *testing.T) {
	type Tag string
	input := "x') OR 1=1 --"
	for _, pair := range [][2]driver.Valuer{{Array([]Tag{Tag(input)}), Array([]string{input})}, {Tuple(struct{ V Tag }{Tag(input)}), Tuple(struct{ V string }{input})}, {Map(map[string]Tag{"k": Tag(input)}), Map(map[string]string{"k": input})}} {
		a, e := pair[0].Value()
		require.NoError(t, e)
		b, e := pair[1].Value()
		require.NoError(t, e)
		require.Equal(t, b, a)
	}
}
func TestPresignIPPolicy(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1", "::ffff:127.0.0.1", "10.1.1.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "0.0.0.0", "ff02::1"} {
		require.False(t, publicPresignIP(net.ParseIP(ip)), ip)
	}
	require.True(t, publicPresignIP(net.ParseIP("8.8.8.8")))
}
func TestPresignTargetAndRedirectPolicy(t *testing.T) {
	var uploads int
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { uploads++; w.WriteHeader(200) }))
	defer storage.Close()
	destination := storage.URL
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "test", "data": [][]string{{"PUT", "{}", destination}}, "schema": []map[string]string{{"name": "method", "type": "String"}, {"name": "headers", "type": "String"}, {"name": "url", "type": "String"}}, "state": "Succeeded"})
	}))
	defer api.Close()
	cfg, e := ParseDSN(strings.Replace(api.URL, "http://", "lake+http://u:p@", 1) + "/?login=disable")
	require.NoError(t, e)
	c := NewAPIClientFromConfig(cfg)
	upload := func() error {
		return c.UploadToStageByPresignURL(context.Background(), &StageLocation{}, bufio.NewReader(strings.NewReader("dummy")), 5)
	}
	require.Error(t, upload())
	require.Zero(t, uploads)
	c.PresignAllowPrivate = true
	require.NoError(t, upload())
	require.Equal(t, 1, uploads)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", storage.URL)
		w.WriteHeader(303)
	}))
	defer redirect.Close()
	destination = redirect.URL
	require.Error(t, upload())
	require.Equal(t, 1, uploads)
}
