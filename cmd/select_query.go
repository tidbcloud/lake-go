package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"reflect"

	"github.com/pkg/errors"

	dc "github.com/tidbcloud/lake-go"
)

func fatalf(format string, args ...interface{}) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

// reportError records diagnostic metadata, not server messages or SQL text.
func reportError(operation string, err error) {
	attrs := []any{"error_type", fmt.Sprintf("%T", err)}
	var apiErr dc.APIError
	if errors.As(err, &apiErr) {
		attrs = append(attrs, "status_code", apiErr.StatusCode)
	}
	var queryErr *dc.QueryError
	if errors.As(err, &queryErr) {
		attrs = append(attrs, "query_error_code", queryErr.Code)
	}
	slog.Error(operation, attrs...)
}

func fatalError(operation string, err error) {
	reportError(operation, err)
	os.Exit(1)
}

// getDSN constructs a DSN based on the test connection parameters
func getDSN() (string, *dc.Config, error) {
	env := func(k string, failOnMissing bool) string {
		if value := os.Getenv(k); value != "" {
			return value
		}
		if failOnMissing {
			fatalf("%v environment variable is not set.", k)
		}
		return ""
	}

	user := env("LAKE_TEST_USER", true)
	password := env("LAKE_TEST_PASSWORD", true)
	warehouse := env("LAKE_TEST_WAREHOUSE", true)
	host := env("LAKE_TEST_HOST", false)
	accessToken := env("LAKE_TEST_ACCESSTOKEN", false)
	var err error
	cfg := dc.NewConfig()
	cfg.Warehouse = warehouse
	cfg.User = user
	cfg.Password = password
	cfg.Host = host
	cfg.Database = "books"
	cfg.AccessToken = accessToken
	cfg.Debug = true

	dsn := cfg.FormatDSN()
	return dsn, cfg, err
}

func main() {
	dsn, _, err := getDSN()
	if err != nil {
		fatalError("failed to create DSN from Config", err)
	}

	db, err := sql.Open("lake", dsn)
	if err != nil {
		fatalError("failed to connect", err)
	}
	err = db.Ping()
	if err != nil {
		fatalError("connection check failed", err)
	}
	defer db.Close()
	query := "DESC books"
	rows, err := db.Query(query) // no cancel is allowed
	if err != nil {
		fatalError("failed to run a query", err)
	}
	res, err := scanValues(rows)
	if err != nil {
		reportError("scan failed", err)
	}
	fmt.Println(res)

	fmt.Println("Query completed successfully.")

	err = selectExec(dsn)
	if err != nil {
		reportError("exec failed", err)
	}
	err = batchInsert(dsn)
	if err != nil {
		reportError("batch insert failed", err)
	}
}
func selectExec(dsn string) error {
	db, err := sql.Open("lake", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer db.Close()
	query := "SELECT ?"

	rows, err := db.Exec(query, []interface{}{1}...) // no cancel is allowed
	if err != nil {
		return errors.Wrap(err, "failed to run a query")
	}
	fmt.Println(rows.RowsAffected())
	fmt.Println("Query completed successfully.")
	return nil
}

func scanValues(rows *sql.Rows) (interface{}, error) {
	var err error
	var result [][]interface{}
	ct, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	types := make([]reflect.Type, len(ct))
	for i, v := range ct {
		types[i] = v.ScanType()
	}
	ptrs := make([]interface{}, len(types))
	for rows.Next() {
		if err = rows.Err(); err != nil {
			return nil, err
		}
		for i, t := range types {
			ptrs[i] = reflect.New(t).Interface()
		}
		err = rows.Scan(ptrs...)
		if err != nil {
			return nil, err
		}
		values := make([]interface{}, len(types))
		for i, p := range ptrs {
			values[i] = reflect.ValueOf(p).Elem().Interface()
		}
		result = append(result, values)
	}
	return result, nil
}

func batchInsert(dsn string) error {
	db, err := sql.Open("lake", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer db.Close()
	scope, err := db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin failed")
	}
	batch, err := scope.Prepare("INSERT INTO books VALUES(?, ?, ?)")
	if err != nil {
		return errors.Wrap(err, "prepare failed")
	}
	for i := 0; i < 10; i++ {
		_, err = batch.Exec(
			"book",
			"author",
			"2022",
		)
		if err != nil {
			return err
		}
	}

	err = scope.Commit()
	if err != nil {
		return err
	}

	return nil
}
