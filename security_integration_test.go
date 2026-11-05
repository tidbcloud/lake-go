package golake

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIntegrationSecurityBindLiterals(t *testing.T) {
	cfg := integrationTestConfig(t)
	db, err := sql.Open("lake", cfg.FormatDSN())
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Both database/sql paths must retain literal question marks and quoted data.
	query := `SELECT ?, 'it\'s', 'q=?', ? /* ? */`
	payload := "UNION SELECT 123 --"
	check := func(rows *sql.Row) {
		var first, literal, question, second string
		require.NoError(t, rows.Scan(&first, &literal, &question, &second))
		require.Equal(t, "a", first)
		require.Equal(t, "it's", literal)
		require.Equal(t, "q=?", question)
		require.Equal(t, payload, second)
	}
	check(db.QueryRowContext(ctx, query, "a", payload))
	stmt, err := db.PrepareContext(ctx, query)
	require.NoError(t, err)
	defer stmt.Close()
	check(stmt.QueryRowContext(ctx, "a", payload))
	var literal, value string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT $$q=?$$, ?`, "x' \\ --").Scan(&literal, &value))
	require.Equal(t, "q=?", literal)
	require.Equal(t, "x' \\ --", value)
}

func TestIntegrationRoleStateRequiresServerGrants(t *testing.T) {
	cfg := integrationTestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin := NewAPIClientFromConfig(cfg)
	suffix := fmt.Sprint(time.Now().UnixNano())
	user, reader, other := "sec_user_"+suffix, "sec_reader_"+suffix, "sec_other_"+suffix
	exec := func(sql string) { _, err := admin.QuerySync(ctx, sql); require.NoError(t, err) }
	exec("CREATE ROLE " + reader)
	defer admin.QuerySync(context.Background(), "DROP ROLE IF EXISTS "+reader)
	exec("CREATE ROLE " + other)
	defer admin.QuerySync(context.Background(), "DROP ROLE IF EXISTS "+other)
	exec("CREATE USER '" + user + "' IDENTIFIED BY 'test-only-password'")
	defer admin.QuerySync(context.Background(), "DROP USER IF EXISTS '"+user+"'")
	exec("GRANT ROLE " + reader + " TO '" + user + "'")
	exec("GRANT ROLE " + other + " TO '" + user + "'")
	limitedConfig := *cfg
	limitedConfig.User, limitedConfig.Password, limitedConfig.Role = user, "test-only-password", reader
	limited := NewAPIClientFromConfig(&limitedConfig)
	resp, err := limited.QuerySync(ctx, "SELECT current_role()")
	require.NoError(t, err)
	actual, ok := resp.cellString(0, 0)
	require.True(t, ok)
	require.Equal(t, reader, actual)
	// A configured role is the initial role, not an immutable privilege ceiling.
	_, err = limited.QuerySync(ctx, "SET ROLE "+other)
	require.NoError(t, err)
	resp, err = limited.QuerySync(ctx, "SELECT current_role()")
	require.NoError(t, err)
	actual, ok = resp.cellString(0, 0)
	require.True(t, ok)
	require.Equal(t, other, actual)
	// A server response cannot grant a role that this authenticated user lacks.
	state := limited.GetState()
	var session map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(state.SessionState), &session))
	session["role"], session["secondary_roles"] = "account_admin", nil
	raw, err := json.Marshal(session)
	require.NoError(t, err)
	message := json.RawMessage(raw)
	limited.applySessionState(&QueryResponse{Session: &message})
	_, err = limited.QuerySync(ctx, "SELECT * FROM system.users")
	require.Error(t, err)
	require.True(t, strings.Contains(strings.ToLower(err.Error()), "role") || strings.Contains(strings.ToLower(err.Error()), "privilege"), err)
	state.SessionState = string(raw)
	restored := NewAPIClientFromConfig(&limitedConfig).WithState(state)
	_, err = restored.QuerySync(ctx, "SELECT * FROM system.users")
	require.Error(t, err)
	require.True(t, strings.Contains(strings.ToLower(err.Error()), "role") || strings.Contains(strings.ToLower(err.Error()), "privilege"), err)
}
