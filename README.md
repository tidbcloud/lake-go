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
format `lake://user:password@host:443/database?<query_option>=<value>`.

Set `LAKE_DSN` before running the examples:

```shell
export LAKE_DSN='lake://user:password@host:443/default?warehouse=your-warehouse'
```

```go
import (
    "database/sql"
    "fmt"
    "os"

    _ "github.com/tidbcloud/lake-go"
)

func openLake() (*sql.DB, error) {
    dsn := os.Getenv("LAKE_DSN")
    if dsn == "" {
        return nil, fmt.Errorf("set LAKE_DSN before running this example")
    }
    return sql.Open("lake", dsn)
}

func ConnectDSN() error {
    conn, err := openLake()
    if err != nil {
        return err
    }
    defer conn.Close()
    return conn.Ping()
}
```

> The runnable snippets in the sections below reuse the `openLake()` helper
> defined above (and assume `fmt` is imported). Each snippet is a function body,
> not a standalone `main`.

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
conn, err := openLake()
if err != nil {
    fmt.Println(err)
    return
}
defer conn.Close()
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
conn, err := openLake()
if err != nil {
    fmt.Println(err)
    return
}
defer conn.Close()

conn.Exec("DROP TABLE IF EXISTS test")
_, err = conn.Exec(`CREATE TABLE test(
    Col1 BIGINT,
    Col2 VARCHAR
)`)
tx, err := conn.Begin()
if err != nil {
    fmt.Println(err)
    return
}
batch, err := tx.Prepare(fmt.Sprintf("INSERT INTO %s VALUES (?, ?)", "test"))
if err != nil {
    fmt.Println(err)
    return
}
for i := 0; i < 10; i++ {
    _, err = batch.Exec(i+1, fmt.Sprintf("row-%d", i+1))
}
err = tx.Commit()
```

## Querying Row/s

Querying a single row can be achieved using the QueryRow method.

```go
conn, err := openLake()
if err != nil {
    fmt.Println(err)
    return
}
defer conn.Close()
row := conn.QueryRow("SELECT Col1, Col2 FROM data")
var (
    col1 int8
    col2 string
)
if err := row.Scan(&col1, &col2); err != nil {
    fmt.Println(err)
}
fmt.Println(col2)
```

Iterating multiple rows requires the Query method.

```go
conn, err := openLake()
if err != nil {
    fmt.Println(err)
    return
}
defer conn.Close()
row, err := conn.Query("SELECT Col1, Col2 FROM data")
if err != nil {
    fmt.Println(err)
    return
}
defer row.Close()
var (
    col1 int8
    col2 string
)
for row.Next() {
    if err := row.Scan(&col1, &col2); err != nil {
        fmt.Println(err)
    }
    fmt.Println(col2)
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
| Binary             | []byte          |
| Decimal            | decimal.Decimal |
| String             | string          |
| Date               | time.Time       |
| DateTime           | time.Time       |
| Array(T)           | string          |
| Tuple(T1, T2, ..) | string          |
| Variant            | string          |

`Binary` is returned as raw `[]byte`. If you scan it into `string`, `database/sql` applies its default `[]byte` to `string` conversion; this does not reformat the value using `binary_output_format`.

## Placeholders and escaping

The driver substitutes `?` placeholders on the client side, so every bind value
is rendered as SQL text before the statement is sent.

Question marks inside quoted strings, quoted identifiers, `$$` strings and SQL
comments are not bind placeholders. Direct and prepared execution use the same
scanner.

- `string` values are escaped and quoted.
- `[]byte` values are passed through as raw SQL text (the same channel the
  `Date`, `UInt64`, `Decimal*` and `Interval` wrappers use). Do not pass
  attacker-controlled data as `[]byte` to a placeholder: it is not escaped and
  is spliced into the statement verbatim. Bind it as a `string` instead.

## Upgrade notes

### Connection and SQL hardening

- Explicit HTTPS DSNs (including `lake+https`) reject `sslmode=disable`.
  Use an explicit HTTP DSN or `lake://...?sslmode=disable` for intentional
  plaintext connections, such as local development. Unknown schemes are rejected.
- Malformed DSN errors and driver logs omit the input URL and credentials;
  parse failures remain `*url.Error` values with a redacted URL and diagnostic.
- `role` selects an initial role. The server validates permissions against the
  authenticated user's grants. SQL can change the current role or secondary
  roles; setting this option does not sandbox an overprivileged credential.
  Resume with signed snapshots created by `GetState`. They are bound to the
  warehouse endpoint, configured user, tenant, warehouse and role. The default
  signature key is process-local. Set a shared, high-entropy `Config.StateSigningKey`
  in trusted application configuration for cross-process resume. Recreate snapshots
  created before this change. Use `RestoreState` for immediate validation errors.
- Configure custom TLS with `RegisterTLSConfig` and the `tls_config` DSN option.
  An unknown configuration, or combining it with `sslmode=disable`, fails at
  connection time.

### 0.9.4

- Server-supplied `next_uri` / `final_uri` / `kill_uri` are validated before
  use. A relative path is used exactly as before; an absolute URL is accepted
  only when its scheme and host:port equal the configured endpoint. Anything
  else - another host such as `https://other.example/v1/query`, a scheme
  downgrade such as `http://` against an `https://` endpoint, a
  protocol-relative `//host/...`, an `@host/...` form, or a non-rooted relative
  path - is rejected instead of being sent, so the warehouse credentials and
  session cookies never go to a host named by the response. The old
  `apiEndpoint + path` concatenation is preserved verbatim for relative
  paths, including the two shapes that also worked before: an empty value
  (the endpoint itself) and a query-only value such as `?page=2`.
- HTTP redirects on query and authenticated stage-upload requests must keep
  the same scheme and host:port as the initial, validated request, including
  every hop in a redirect chain. Same-endpoint redirects remain supported
  with the existing ten-request limit.
- A rejected URI is still classified as a request failure: the error satisfies
  `errors.Is(err, golake.ErrDoRequest)` and keeps the message
  `refusing to send a request to ...`, so callers that retry or classify on
  that sentinel keep matching. It is not retried with the previous fixed-delay
  retry loop, because retrying a deterministically refused URI only delays the
  error.
- `Config` and `APIClient` now implement `fmt.Stringer`: printing them with
  `%v`, `%+v` or `%s` yields one line in which `password` and `access_token`
  are replaced by `<redacted>` (or stay empty when unset). Code that parsed the
  old raw struct dump must read the fields directly (for example
  `cfg.Password`) or call `FormatDSN()`.
- `APIError` diagnostic formatting and structured logging show the HTTP status
  and caller-provided hint, without the upstream response text or message.
  `RespText`, `RespBody` and the `RespBody(err)` accessor retain the original
  response for programmatic use; do not log or serialize those fields without
  redaction. Inspect these fields rather than parsing the diagnostic string.
  Warehouse-provisioning timeout retries still inspect the response explicitly.
  The bundled command examples log failure metadata rather than SQL or raw
  errors. Query result output remains part of their explicit command output.
- `Decimal32` / `Decimal64` / `Decimal128` values are escaped before they are
  interpolated into the `toDecimalN('...', s)` literal. Valid decimal values
  still produce the same SQL as before; a value containing `'` or `\` can no
  longer close the literal and append SQL, so a malformed value now makes the
  server return an error instead of performing an injection or silently
  corrupting the statement.

### Connection and binding follow-up

DSN userinfo is parsed only inside the URL authority. Encode structural characters
(`/`, `?`, `#`, `@`) in usernames/passwords, preferably using `url.UserPassword`;
raw separators are URL structure and are no longer reinterpreted as credentials.
Named Go string types inside Array/Tuple/Map use the same escaping as string.
The explicit raw `[]byte` channel and typed SQL wrappers retain their behavior.

Presigned uploads require HTTPS and public destination addresses by default.
DNS answers are checked at connection time and the checked IP is dialed directly;
proxies and redirects are disabled. For a trusted private object store/local
MinIO, explicitly set `presign_allow_private=true`. That opt-in permits private
addresses and HTTP, but still does not follow redirects. Storage remains a
separate endpoint from the Lake API, so public S3/OSS presigned URLs are supported.
