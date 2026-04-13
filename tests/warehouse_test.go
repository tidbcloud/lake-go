package tests

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dc "github.com/tidbcloud/lake-go"
)

// TestWarehouseSuite tests lake-go against a real TiDB Cloud Lake warehouse.
// Set TEST_LAKE_DSN env to run:
//
//	TEST_LAKE_DSN="lake://user:pass@host:443/default?warehouse=wh" go test -v -run TestWarehouseSuite ./tests/
func TestWarehouseSuite(t *testing.T) {
	d := os.Getenv("TEST_LAKE_DSN")
	if d == "" {
		t.Skip("TEST_LAKE_DSN not set, skipping warehouse integration tests")
	}
	s := &WarehouseSuite{dsn: d}
	s.setup(t)
	defer s.cleanup(t)

	t.Run("Ping", s.TestPing)
	t.Run("SelectOne", s.TestSelectOne)
	t.Run("ServerVersion", s.TestServerVersion)
	t.Run("ShowDatabases", s.TestShowDatabases)
	t.Run("CreateDropDatabase", s.TestCreateDropDatabase)
	t.Run("DDL_CRUD", s.TestDDLCRUD)
	t.Run("TypeMapping", s.TestTypeMapping)
	t.Run("NullHandling", s.TestNullHandling)
	t.Run("MultiPageResult", s.TestMultiPageResult)
	t.Run("Prepare", s.TestPrepare)
	t.Run("Transaction", s.TestTransaction)
	t.Run("BatchInsert", s.TestBatchInsert)
	t.Run("SessionConfig", s.TestSessionConfig)
	t.Run("ContextTimeout", s.TestContextTimeout)
	t.Run("ErrorHandling", s.TestErrorHandling)
	t.Run("DSNParsing", s.TestDSNParsing)
	t.Run("ConcurrentQueries", s.TestConcurrentQueries)
}

type WarehouseSuite struct {
	dsn  string
	cfg  *dc.Config
	db   *sql.DB
	tbl  string // per-run test table
	dbID string // per-run test database
}

func (s *WarehouseSuite) setup(t *testing.T) {
	t.Helper()
	var err error
	s.cfg, err = dc.ParseDSN(s.dsn)
	require.NoError(t, err)

	s.db, err = sql.Open("lake", s.dsn)
	require.NoError(t, err)
	require.NoError(t, s.db.Ping())

	// Switch to 'default' database for DDL tests (information_schema is read-only)
	_, err = s.db.Exec("USE default")
	require.NoError(t, err)

	ts := time.Now().Unix()
	s.tbl = fmt.Sprintf("lake_test_%d", ts)
	s.dbID = fmt.Sprintf("lake_testdb_%d", ts)
}

func (s *WarehouseSuite) cleanup(t *testing.T) {
	t.Helper()
	s.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", s.tbl))
	s.db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", s.dbID))
	s.db.Close()
}

// --- Basic connectivity ---

func (s *WarehouseSuite) TestPing(t *testing.T) {
	assert.NoError(t, s.db.Ping())
}

func (s *WarehouseSuite) TestSelectOne(t *testing.T) {
	var v int
	err := s.db.QueryRow("SELECT 1").Scan(&v)
	assert.NoError(t, err)
	assert.Equal(t, 1, v)
}

func (s *WarehouseSuite) TestServerVersion(t *testing.T) {
	var ver string
	err := s.db.QueryRow("SELECT version()").Scan(&ver)
	assert.NoError(t, err)
	assert.NotEmpty(t, ver)
	t.Logf("Server version: %s", ver)
}

func (s *WarehouseSuite) TestShowDatabases(t *testing.T) {
	rows, err := s.db.Query("SHOW DATABASES")
	require.NoError(t, err)
	defer rows.Close()

	var dbs []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		dbs = append(dbs, name)
	}
	assert.Contains(t, dbs, "default")
	assert.Contains(t, dbs, "information_schema")
	assert.Contains(t, dbs, "system")
	t.Logf("Found %d databases: %v", len(dbs), dbs)
}

// --- DDL & CRUD ---

func (s *WarehouseSuite) TestCreateDropDatabase(t *testing.T) {
	r := require.New(t)
	_, err := s.db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", s.dbID))
	r.NoError(err)

	rows, err := s.db.Query("SHOW DATABASES")
	r.NoError(err)
	defer rows.Close()

	found := false
	for rows.Next() {
		var name string
		rows.Scan(&name)
		if name == s.dbID {
			found = true
		}
	}
	assert.True(t, found, "database %s should exist after CREATE", s.dbID)

	_, err = s.db.Exec(fmt.Sprintf("DROP DATABASE %s", s.dbID))
	assert.NoError(t, err)
}

func (s *WarehouseSuite) TestDDLCRUD(t *testing.T) {
	r := require.New(t)
	tbl := s.tbl

	// CREATE
	_, err := s.db.Exec(fmt.Sprintf(`CREATE TABLE %s (
		id      Int64,
		name    String,
		score   Float64,
		created Date
	)`, tbl))
	r.NoError(err)

	// DESC
	rows, err := s.db.Query("DESC " + tbl)
	r.NoError(err)
	cols, _ := scanValues(rows)
	r.Len(cols, 4)
	r.Equal("id", cols[0][0])
	r.Equal("name", cols[1][0])

	// INSERT
	result, err := s.db.Exec(
		fmt.Sprintf("INSERT INTO %s VALUES (?, ?, ?, ?)", tbl),
		int64(1), "alice", 95.5, "2024-01-15",
	)
	r.NoError(err)
	n, _ := result.RowsAffected()
	r.Equal(int64(1), n)

	// Insert more rows
	for i := 2; i <= 5; i++ {
		_, err = s.db.Exec(
			fmt.Sprintf("INSERT INTO %s VALUES (?, ?, ?, ?)", tbl),
			int64(i), fmt.Sprintf("user%d", i), float64(80+i), "2024-02-01",
		)
		r.NoError(err)
	}

	// SELECT with WHERE
	var name string
	var score float64
	err = s.db.QueryRow(
		fmt.Sprintf("SELECT name, score FROM %s WHERE id = ?", tbl), 1,
	).Scan(&name, &score)
	r.NoError(err)
	r.Equal("alice", name)
	r.Equal(95.5, score)

	// COUNT
	var count int
	err = s.db.QueryRow(fmt.Sprintf("SELECT count(*) FROM %s", tbl)).Scan(&count)
	r.NoError(err)
	r.Equal(5, count)

	// DELETE
	_, err = s.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = 1", tbl))
	r.NoError(err)

	err = s.db.QueryRow(fmt.Sprintf("SELECT count(*) FROM %s", tbl)).Scan(&count)
	r.NoError(err)
	r.Equal(4, count)

	// DROP
	_, err = s.db.Exec(fmt.Sprintf("DROP TABLE %s", tbl))
	r.NoError(err)
}

// --- Type mapping ---

func (s *WarehouseSuite) TestTypeMapping(t *testing.T) {
	r := require.New(t)
	tbl := fmt.Sprintf("type_test_%d", time.Now().Unix())

	_, err := s.db.Exec(fmt.Sprintf(`CREATE TABLE %s (
		c_int8    TINYINT,
		c_int16   SMALLINT,
		c_int32   INT,
		c_int64   BIGINT,
		c_uint8   TINYINT UNSIGNED,
		c_uint64  BIGINT UNSIGNED,
		c_f32     Float32,
		c_f64     Float64,
		c_str     String,
		c_date    Date,
		c_dt      DateTime,
		c_bool    Boolean,
		c_arr     Array(Int32)
	)`, tbl))
	r.NoError(err)
	defer s.db.Exec(fmt.Sprintf("DROP TABLE %s", tbl))

	_, err = s.db.Exec(fmt.Sprintf(`INSERT INTO %s VALUES
		(-1, -2, -3, -4, 5, 6, 1.5, 2.5, 'hello', '2024-06-15', '2024-06-15 10:30:00', true, [1,2,3])`, tbl))
	r.NoError(err)

	rows, err := s.db.Query(fmt.Sprintf("SELECT * FROM %s", tbl))
	r.NoError(err)
	defer rows.Close()

	colTypes, err := rows.ColumnTypes()
	r.NoError(err)
	t.Log("Column types:")
	for _, ct := range colTypes {
		t.Logf("  %s: %s", ct.Name(), ct.DatabaseTypeName())
	}

	r.True(rows.Next())
	vals := make([]interface{}, len(colTypes))
	ptrs := make([]interface{}, len(colTypes))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	r.NoError(rows.Scan(ptrs...))

	t.Log("Values:")
	for i, ct := range colTypes {
		t.Logf("  %s = %v (Go type: %T)", ct.Name(), vals[i], vals[i])
	}
}

// --- Null handling ---

func (s *WarehouseSuite) TestNullHandling(t *testing.T) {
	r := require.New(t)
	tbl := fmt.Sprintf("null_test_%d", time.Now().Unix())

	_, err := s.db.Exec(fmt.Sprintf(`CREATE TABLE %s (
		id  Int64,
		val String NULL
	)`, tbl))
	r.NoError(err)
	defer s.db.Exec(fmt.Sprintf("DROP TABLE %s", tbl))

	_, err = s.db.Exec(fmt.Sprintf("INSERT INTO %s VALUES (1, 'abc')", tbl))
	r.NoError(err)
	_, err = s.db.Exec(fmt.Sprintf("INSERT INTO %s VALUES (2, NULL)", tbl))
	r.NoError(err)

	rows, err := s.db.Query(fmt.Sprintf("SELECT id, val FROM %s ORDER BY id", tbl))
	r.NoError(err)
	defer rows.Close()

	// Row 1: non-null
	r.True(rows.Next())
	var id int64
	var val sql.NullString
	r.NoError(rows.Scan(&id, &val))
	r.Equal(int64(1), id)
	r.True(val.Valid)
	r.Equal("abc", val.String)

	// Row 2: null
	r.True(rows.Next())
	r.NoError(rows.Scan(&id, &val))
	r.Equal(int64(2), id)
	r.False(val.Valid)
}

// --- Multi-page result ---

func (s *WarehouseSuite) TestMultiPageResult(t *testing.T) {
	r := require.New(t)
	n := 25000 // larger than default page size (10000)

	rows, err := s.db.Query(fmt.Sprintf("SELECT number FROM numbers(%d) ORDER BY number", n))
	r.NoError(err)
	defer rows.Close()

	count := 0
	prev := -1
	for rows.Next() {
		var v int
		r.NoError(rows.Scan(&v))
		r.Equal(prev+1, v, "results should be sequential")
		prev = v
		count++
	}
	r.Equal(n, count)
	t.Logf("Successfully read %d rows across multiple pages", count)
}

// --- Prepared statements ---

func (s *WarehouseSuite) TestPrepare(t *testing.T) {
	r := require.New(t)
	tbl := fmt.Sprintf("prep_test_%d", time.Now().Unix())

	_, err := s.db.Exec(fmt.Sprintf("CREATE TABLE %s (a String)", tbl))
	r.NoError(err)
	defer s.db.Exec(fmt.Sprintf("DROP TABLE %s", tbl))

	stmt, err := s.db.Prepare(fmt.Sprintf("INSERT INTO %s VALUES (?)", tbl))
	r.NoError(err)
	defer stmt.Close()

	for i := 0; i < 5; i++ {
		_, err = stmt.Exec(fmt.Sprintf("val_%d", i))
		r.NoError(err)
	}

	var count int
	err = s.db.QueryRow(fmt.Sprintf("SELECT count(*) FROM %s", tbl)).Scan(&count)
	r.NoError(err)
	r.Equal(5, count)

	// Prepared query
	stmt2, err := s.db.Prepare(fmt.Sprintf("SELECT a FROM %s WHERE a = ?", tbl))
	r.NoError(err)
	defer stmt2.Close()

	var result string
	err = stmt2.QueryRow("val_3").Scan(&result)
	r.NoError(err)
	r.Equal("val_3", result)
}

// --- Transactions ---

func (s *WarehouseSuite) TestTransaction(t *testing.T) {
	r := require.New(t)
	tbl := fmt.Sprintf("txn_test_%d", time.Now().Unix())

	_, err := s.db.Exec(fmt.Sprintf("CREATE TABLE %s (id Int64, val String)", tbl))
	r.NoError(err)
	defer s.db.Exec(fmt.Sprintf("DROP TABLE %s", tbl))

	// Test COMMIT
	tx, err := s.db.Begin()
	r.NoError(err)
	_, err = tx.Exec(fmt.Sprintf("INSERT INTO %s VALUES (1, 'committed')", tbl))
	r.NoError(err)
	r.NoError(tx.Commit())

	var val string
	err = s.db.QueryRow(fmt.Sprintf("SELECT val FROM %s WHERE id = 1", tbl)).Scan(&val)
	r.NoError(err)
	r.Equal("committed", val)

	// Test ROLLBACK
	tx2, err := s.db.Begin()
	r.NoError(err)
	_, err = tx2.Exec(fmt.Sprintf("INSERT INTO %s VALUES (2, 'rolled_back')", tbl))
	r.NoError(err)
	r.NoError(tx2.Rollback())

	var count int
	err = s.db.QueryRow(fmt.Sprintf("SELECT count(*) FROM %s WHERE id = 2", tbl)).Scan(&count)
	r.NoError(err)
	r.Equal(0, count, "rolled back row should not exist")
}

// --- Batch insert ---

func (s *WarehouseSuite) TestBatchInsert(t *testing.T) {
	r := require.New(t)
	tbl := fmt.Sprintf("batch_test_%d", time.Now().Unix())

	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	r.NoError(err)
	defer conn.Close()

	_, err = conn.ExecContext(ctx, "USE default")
	r.NoError(err)

	_, err = conn.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE %s (
		id  Int64,
		val String,
		ts  DateTime
	)`, tbl))
	r.NoError(err)
	defer s.db.Exec(fmt.Sprintf("DROP TABLE %s", tbl))

	// Use batch via PrepareBatch + ExecBatch
	query := fmt.Sprintf("INSERT INTO %s VALUES", tbl)
	stmt, err := dc.PrepareBatch(query)
	r.NoError(err)

	now := time.Now().UTC().Truncate(time.Second).Format("2006-01-02 15:04:05")
	var batch [][]driver.Value
	for i := 0; i < 20; i++ {
		batch = append(batch, []driver.Value{
			int64(i),
			fmt.Sprintf("batch_val_%d", i),
			now,
		})
	}

	result, err := stmt.ExecBatch(ctx, conn, batch)
	r.NoError(err)
	n, err := result.RowsAffected()
	r.NoError(err)
	r.Equal(int64(20), n)

	var count int
	err = conn.QueryRowContext(ctx, fmt.Sprintf("SELECT count(*) FROM %s", tbl)).Scan(&count)
	r.NoError(err)
	r.Equal(20, count)
	t.Logf("Batch inserted %d rows", count)
}

// --- Session config ---

func (s *WarehouseSuite) TestSessionConfig(t *testing.T) {
	r := require.New(t)

	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	r.NoError(err)
	defer conn.Close()

	// Switch to default first
	_, err = conn.ExecContext(ctx, "USE default")
	r.NoError(err)

	var dbName string
	err = conn.QueryRowContext(ctx, "SELECT currentDatabase()").Scan(&dbName)
	r.NoError(err)
	r.Equal("default", dbName)

	// Switch to system
	_, err = conn.ExecContext(ctx, "USE system")
	r.NoError(err)
	err = conn.QueryRowContext(ctx, "SELECT currentDatabase()").Scan(&dbName)
	r.NoError(err)
	r.Equal("system", dbName)

	// Switch back to default
	_, err = conn.ExecContext(ctx, "USE default")
	r.NoError(err)
	err = conn.QueryRowContext(ctx, "SELECT currentDatabase()").Scan(&dbName)
	r.NoError(err)
	r.Equal("default", dbName)
}

// --- Context timeout ---

func (s *WarehouseSuite) TestContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, "SELECT number FROM numbers(1000) ORDER BY number")
	require.NoError(t, err)
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	assert.Equal(t, 1000, count)
}

// --- Error handling ---

func (s *WarehouseSuite) TestErrorHandling(t *testing.T) {
	// Invalid SQL
	_, err := s.db.Query("SELECTX invalid_sql")
	assert.Error(t, err)
	t.Logf("Expected error for invalid SQL: %v", err)

	// Non-existent table
	_, err = s.db.Query("SELECT * FROM nonexistent_table_xyz_12345")
	assert.Error(t, err)
	t.Logf("Expected error for missing table: %v", err)
}

// --- DSN parsing ---

func (s *WarehouseSuite) TestDSNParsing(t *testing.T) {
	r := require.New(t)

	cfg, err := dc.ParseDSN(s.dsn)
	r.NoError(err)
	r.NotEmpty(cfg.Host)
	r.NotEmpty(cfg.User)
	t.Logf("DSN parsed: host=%s, user=%s, db=%s, warehouse=%s",
		cfg.Host, cfg.User, cfg.Database, cfg.Warehouse)

	// Round-trip: parse -> format -> parse
	formatted := cfg.FormatDSN()
	cfg2, err := dc.ParseDSN(formatted)
	r.NoError(err)
	r.Equal(cfg.Host, cfg2.Host)
	r.Equal(cfg.User, cfg2.User)
	r.Equal(cfg.Database, cfg2.Database)
	r.Equal(cfg.Warehouse, cfg2.Warehouse)
}

// --- Concurrent queries ---

func (s *WarehouseSuite) TestConcurrentQueries(t *testing.T) {
	r := require.New(t)
	const numGoroutines = 5

	errCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			var v int
			err := s.db.QueryRow("SELECT ?", id).Scan(&v)
			if err != nil {
				errCh <- fmt.Errorf("goroutine %d: %w", id, err)
				return
			}
			if v != id {
				errCh <- fmt.Errorf("goroutine %d: expected %d got %d", id, id, v)
				return
			}
			errCh <- nil
		}(i)
	}

	for i := 0; i < numGoroutines; i++ {
		err := <-errCh
		r.NoError(err)
	}
	t.Logf("All %d concurrent queries succeeded", numGoroutines)
}
