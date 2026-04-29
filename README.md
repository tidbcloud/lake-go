# lake-go

Golang driver for [TiDB Cloud Lake](https://tidbcloud.com/)

## Installation

```
go get github.com/tidbcloud/lake-go
```

## Key features

- Supports native Lake HTTP client-server protocol
- Compatibility with [`database/sql`](#std-databasesql-interface)

# Examples

## Connecting

Connection can be achieved either via a DSN string with the
format `https://user:password@host/database?<query_option>=<value>` and sql/Open method.

```go
import (
    "database/sql"
    _ "github.com/tidbcloud/lake-go"
)

func ConnectDSN() error {
    dsn, cfg, err := getDSN()
    if err != nil {
        log.Fatalf("failed to create DSN from Config: %v, err: %v", cfg, err)
    }
    conn, err := sql.Open("lake", dsn)
    if err != nil {
        return err
    }
    return conn.Ping()
}
```

## Connection Settings

You can get the connection settings from the TiDB Cloud console.

- host - the connect host from TiDB Cloud connection page
- username/password - auth credentials from TiDB Cloud connection page
- database - select the current default database

## Query Result Transport

The driver uses JSON query results by default.

To opt in to the HTTP Arrow transport while keeping the public `database/sql` interface unchanged,
set `query_result_format=arrow` in the DSN or set `cfg.QueryResultFormat = golake.QueryResultFormatArrow`.

When Arrow is enabled, the driver only uses it if the server version is `>= 1.2.899`.
If the backend still returns JSON for a query, the driver transparently falls back to the existing row path.

## Execution

Once a connection has been obtained, users can issue sql statements for execution via the Exec method.

```go
dsn, cfg, err := getDSN()
if err != nil {
    log.Fatalf("failed to create DSN from Config: %v, err: %v", cfg, err)
}
conn, err := sql.Open("lake", dsn)
if err != nil {
    fmt.Println(err)
}
conn.Exec(`DROP TABLE IF EXISTS data`)
_, err = conn.Exec(`
    CREATE TABLE IF NOT EXISTS data(
        Col1 TINYINT,
        Col2 VARCHAR
    )`)
if err != nil {
    fmt.Println(err)
}
_, err = conn.Exec("INSERT INTO data VALUES (1, 'test-1')")
```

## Batch Insert

```go
package main

import (
    "database/sql"
    "fmt"

    _ "github.com/tidbcloud/lake-go"
)

func main() {
    conn, err := sql.Open("lake", "lake://user:password@lake.tidbcloud.com/default?sslmode=disable")
    tx, err := conn.Begin()
    if err != nil {
        fmt.Println(err)
    }
    batch, err := tx.Prepare(fmt.Sprintf("INSERT INTO %s VALUES", "test"))
    for i := 0; i < 10; i++ {
        _, err = batch.Exec(
            "1234", "2345", "3.1415", "test", "test2",
            "[4, 5, 6]", "[1, 2, 3]", "2021-01-01", "2021-01-01 00:00:00",
        )
    }
    err = tx.Commit()
}
```

## Querying Row/s

Querying a single row can be achieved using the QueryRow method.

```go
package main

import (
    "database/sql"
    "fmt"

    _ "github.com/tidbcloud/lake-go"
)

func main() {
    conn, err := sql.Open("lake", "lake://user:password@lake.tidbcloud.com/default?sslmode=disable")
    if err != nil {
        fmt.Println(err)
    }
    row := conn.QueryRow("SELECT * FROM data")
    var (
        col1 uint8
        col2 string
    )
    if err := row.Scan(&col1, &col2); err != nil {
        fmt.Println(err)
    }
    fmt.Println(col2)
}
```

Iterating multiple rows requires the Query method.

```go
package main

import (
    "database/sql"
    "fmt"

    _ "github.com/tidbcloud/lake-go"
)

func main() {
    conn, err := sql.Open("lake", "lake://user:password@lake.tidbcloud.com/default?sslmode=disable")
    if err != nil {
        fmt.Println(err)
    }
    row, err := conn.Query("SELECT * FROM data")
    var (
        col1 uint8
        col2 string
    )
    for row.Next() {
        if err := row.Scan(&col1, &col2); err != nil {
            fmt.Println(err)
        }
        fmt.Println(col2)
    }
}
```

## Type Mapping

| Lake Type          | Go Type         |
|--------------------|-----------------|
| TINYINT            | int8            |
| SMALLINT           | int16           |
| INT                | int32           |
| BIGINT             | int64           |
| TINYINT UNSIGNED   | uint8           |
| SMALLINT UNSIGNED  | uint16          |
| INT UNSIGNED       | uint32          |
| BIGINT UNSIGNED    | uint64          |
| Float32            | float32         |
| Float64            | float64         |
| Bitmap             | string          |
| Decimal            | decimal.Decimal |
| String             | string          |
| Date               | time.Time       |
| DateTime           | time.Time       |
| Array(T)           | string          |
| Tuple(T1, T2, ..) | string          |
| Variant            | string          |
