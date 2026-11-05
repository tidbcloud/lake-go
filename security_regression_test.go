package golake

import (
	"bytes"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDSNSecurity(t *testing.T) {
	for _, scheme := range []string{"https", "lake+https", "db+https", "dd+https", "bend+https"} {
		t.Run(scheme, func(t *testing.T) {
			cfg, err := ParseDSN(scheme + "://user:password@localhost/db?sslmode=disable")
			require.Error(t, err)
			require.Nil(t, cfg)
			cfg, err = ParseDSN(scheme + "://user:password@localhost/db")
			require.NoError(t, err)
			require.Equal(t, "https://localhost:443", NewAPIClientFromConfig(cfg).apiEndpoint)
		})
	}
	for _, scheme := range []string{"xhttp", "myhttp", "unknown+http"} {
		_, err := ParseDSN(scheme + "://user:password@localhost/db")
		require.Error(t, err)
	}
	for _, scheme := range []string{"http", "lake+http", "db+http", "dd+http", "bend+http", "lake", "db", "dd", "bend"} {
		cfg, err := ParseDSN(scheme + "://user:password@localhost/db?sslmode=disable")
		require.NoError(t, err)
		require.Equal(t, "http://localhost:80", NewAPIClientFromConfig(cfg).apiEndpoint)
	}
}

func TestMalformedDSNDoesNotLeakIntoLogsOrErrors(t *testing.T) {
	var output bytes.Buffer
	log := CreateDefaultLogger()
	require.NoError(t, log.SetLogLevel("error"))
	log.SetOutput(&output)
	previous := GetLogger()
	SetLogger(log)
	defer SetLogger(previous)
	for _, malformed := range []string{
		"lake://canary-user:canary-password@localhost:bad/db?access_token=canary-token",
		"lake://canary-user:canary-password@localhost/db\n?access_token=canary-token",
	} {
		_, err := ParseDSN(malformed)
		require.Error(t, err)
		var parseErr *url.Error
		require.True(t, errors.As(err, &parseErr))
		combined := output.String() + fmt.Sprint(err) + fmt.Sprint(parseErr.Err)
		for _, secret := range []string{"canary-user", "canary-password", "canary-token"} {
			require.NotContains(t, combined, secret)
		}
	}
	require.Contains(t, output.String(), "invalid DSN URL")
}

func TestPlaceholderLexerSecurity(t *testing.T) {
	tests := []struct {
		query, expected string
		args            []driver.Value
	}{
		{`SELECT ?, 'it\'s', 'q=?', ?`, `SELECT 'a', 'it\'s', 'q=?', 'b'`, []driver.Value{"a", "b"}},
		{`UPDATE notes SET body = ? WHERE author = 'O\'Brien' AND id = ?`, `UPDATE notes SET body = 'a' WHERE author = 'O\'Brien' AND id = 1`, []driver.Value{"a", int64(1)}},
		{`SELECT 'it''s ?', "q=?", ` + "`q=?`" + `, ?`, `SELECT 'it''s ?', "q=?", ` + "`q=?`" + `, 1`, []driver.Value{int64(1)}},
		{"SELECT ? -- '?'\n, ? /* ? ' */", "SELECT 1 -- '?'\n, 2 /* ? ' */", []driver.Value{int64(1), int64(2)}},
		{"SELECT 1 -- ?\f, ?", "SELECT 1 -- ?\f, 2", []driver.Value{int64(2)}},
		{`SELECT $$? ' "$$, ?`, `SELECT $$? ' "$$, 'b'`, []driver.Value{"b"}},
		{`SELECT ? FROM @stage/path?.csv`, `SELECT 1 FROM @stage/path?.csv`, []driver.Value{int64(1)}},
		{`SELECT '資料', ?, ?`, `SELECT '資料', NULL, 'b'`, []driver.Value{nil, "b"}},
		{`SELECT ?, 'q=?'`, `SELECT 'UNION SELECT secret --', 'q=?'`, []driver.Value{"UNION SELECT secret --"}},
	}
	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			require.NotPanics(t, func() {
				actual, err := buildQuery(test.query, test.args, nil)
				require.NoError(t, err)
				require.Equal(t, test.expected, actual)
				offsets := placeholders(test.query)
				actual, err = buildQuery(test.query, test.args, &offsets)
				require.NoError(t, err)
				require.Equal(t, test.expected, actual)
			})
		})
	}
	for _, offsets := range [][]int{{-1}, {10}, {0, 0}, {2, 0}, {1}} {
		args := make([]driver.Value, len(offsets))
		_, err := interpolateParams2("? ?", args, offsets)
		require.ErrorIs(t, err, ErrPlaceholderCount)
	}
}

func FuzzPlaceholderLexerDoesNotPanic(f *testing.F) {
	for _, sql := range []string{`SELECT ?, 'it\'s', 'q=?', ?`, `SELECT \?`, `SELECT "?", $$?$$, ?`, "SELECT ? -- ?\n", "資料 ?"} {
		f.Add(sql)
	}
	f.Fuzz(func(t *testing.T, sql string) {
		if len(sql) > 65536 {
			t.Skip()
		}
		offsets := placeholders(sql)
		values := make([]driver.Value, len(offsets))
		for i := range values {
			values[i] = "' ? \\ --"
		}
		_, err := interpolateParams2(sql, values, offsets)
		require.NoError(t, err)
		previous := -1
		for _, offset := range offsets {
			require.Greater(t, offset, previous)
			require.Equal(t, byte('?'), sql[offset])
			previous = offset
		}
	})
}

// Keep the package's literal escaper assumption visible to the real-engine tests.
func TestEscapedValueRemainsOneLiteral(t *testing.T) {
	value := "x' UNION SELECT 123 --"
	rendered, err := interpolateParams("SELECT ?", []driver.Value{value})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(rendered, "SELECT '"))
	require.Equal(t, `SELECT 'x\' UNION SELECT 123 --'`, rendered)
}
