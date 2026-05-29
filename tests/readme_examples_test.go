package tests

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	_ "github.com/tidbcloud/lake-go"
)

func TestReadmeExamples(t *testing.T) {
	dsn := os.Getenv("TEST_LAKE_DSN")
	if dsn == "" {
		t.Skip("TEST_LAKE_DSN not set, skipping README example validation")
	}

	db, err := sql.Open("lake", dsn)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.Ping())

	execTable := fmt.Sprintf("readme_exec_%d", time.Now().UnixNano())
	batchTable := fmt.Sprintf("readme_batch_%d", time.Now().UnixNano())
	defer db.Exec("DROP TABLE IF EXISTS " + execTable)
	defer db.Exec("DROP TABLE IF EXISTS " + batchTable)

	t.Run("Execution", func(t *testing.T) {
		r := require.New(t)
		_, err := db.Exec("DROP TABLE IF EXISTS " + execTable)
		r.NoError(err)
		_, err = db.Exec(fmt.Sprintf(`CREATE TABLE %s(
			Col1 TINYINT,
			Col2 VARCHAR
		)`, execTable))
		r.NoError(err)
		_, err = db.Exec(fmt.Sprintf("INSERT INTO %s VALUES (1, 'test-1')", execTable))
		r.NoError(err)
	})

	t.Run("BatchInsert", func(t *testing.T) {
		r := require.New(t)
		_, err := db.Exec("DROP TABLE IF EXISTS " + batchTable)
		r.NoError(err)
		_, err = db.Exec(fmt.Sprintf(`CREATE TABLE %s(
			Col1 BIGINT,
			Col2 VARCHAR
		)`, batchTable))
		r.NoError(err)

		tx, err := db.Begin()
		r.NoError(err)
		stmt, err := tx.Prepare(fmt.Sprintf("INSERT INTO %s VALUES (?, ?)", batchTable))
		r.NoError(err)
		defer stmt.Close()

		for i := 0; i < 3; i++ {
			_, err = stmt.Exec(i+1, fmt.Sprintf("batch-%d", i+1))
			r.NoError(err)
		}
		r.NoError(tx.Commit())

		var count int
		r.NoError(db.QueryRow(fmt.Sprintf("SELECT count(*) FROM %s", batchTable)).Scan(&count))
		r.Equal(3, count)
	})

	t.Run("QueryRow", func(t *testing.T) {
		r := require.New(t)
		row := db.QueryRow(fmt.Sprintf("SELECT Col1, Col2 FROM %s ORDER BY Col1 LIMIT 1", execTable))
		var col1 int8
		var col2 string
		r.NoError(row.Scan(&col1, &col2))
		r.Equal(int8(1), col1)
		r.Equal("test-1", col2)
	})

	t.Run("QueryRows", func(t *testing.T) {
		r := require.New(t)
		rows, err := db.Query(fmt.Sprintf("SELECT Col1, Col2 FROM %s ORDER BY Col1", execTable))
		r.NoError(err)
		defer rows.Close()

		var count int
		for rows.Next() {
			var col1 int8
			var col2 string
			r.NoError(rows.Scan(&col1, &col2))
			count++
		}
		r.NoError(rows.Err())
		r.Equal(1, count)
	})
}
