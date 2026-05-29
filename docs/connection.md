# Golang Driver

The Lake Golang driver connects to TiDB Cloud Lake (or a self-hosted Lake
distribution) over its REST API.

## DSN

The DSN format is:

```
lake://<user>:<password>@<host>:443/<database>?warehouse=<your-warehouse>&<option>=<value>
```

TiDB Cloud Lake example (TLS is enabled by default, port is `443`, and a
`warehouse` is required):

```
lake://<user>:<password>@<gateway-host>:443/default?warehouse=<your-warehouse>
```

Self-hosted / local Lake example over plaintext HTTP:

```
lake://root:root@127.0.0.1:8000/default?sslmode=disable&debug=1
```

> You can obtain the host, user, password and warehouse from the TiDB Cloud
> console connection page. Never commit real credentials.

## Connection Parameters

| Parameter                | Description                                                                                                                   | Default          | DSN Example                                                              |
|--------------------------|-------------------------------------------------------------------------------------------------------------------------------|------------------|--------------------------------------------------------------------------|
| user                     | Lake user name (part of the userinfo)                                                                                         | none             | `lake://{user}:pass@host:443/`                                           |
| password                 | Lake user password (part of the userinfo)                                                                                     | none             | `lake://user:{password}@host:443/`                                       |
| database                 | Default database, taken from the URL path                                                                                     | none             | `lake://user:pass@host:443/{database}`                                   |
| warehouse                | Target warehouse. **Required for TiDB Cloud Lake.**                                                                           | none             | `...?warehouse=my-warehouse`                                             |
| tenant                   | Tenant identifier                                                                                                             | none             | `...?tenant=my-tenant`                                                   |
| role                     | Role to use for the connection (limits effective privileges; can be overridden by `SET SECONDARY ROLES ALL`)                 | none             | `...?role=my-role`                                                       |
| sslmode                  | TLS mode. Any value other than `disable` uses TLS (`https`); `disable` uses plaintext (`http`).                              | TLS enabled      | `...?sslmode=disable`                                                    |
| query_result_format      | Result transport: `json` or `arrow`. Arrow is only used when the server version is `>= 1.2.899`, otherwise it falls back to JSON. | `json`           | `...?query_result_format=arrow`                                         |
| access_token             | Static access token (alternative to user/password)                                                                           | none             | `...?access_token=xxx`                                                   |
| access_token_file        | Path to a file containing the access token (supports rotation)                                                               | none             | `...?access_token_file=/path/token`                                      |
| timeout                  | HTTP client timeout (Go duration string)                                                                                     | none (no limit)  | `...?timeout=30s`                                                        |
| wait_time_secs           | RESTful query API blocking time; if the query is not finished, the API blocks up to this many seconds before returning       | server default   | `...?wait_time_secs=10`                                                  |
| max_rows_in_buffer       | The maximum rows kept in the server session buffer                                                                           | server default   | `...?max_rows_in_buffer=50000`                                           |
| max_rows_per_page        | The maximum rows per page in a response body                                                                                 | server default   | `...?max_rows_per_page=100000`                                           |
| presigned_url_disabled   | Set to `true` when the storage layer (e.g. HDFS, local fs) does not support presigned URLs for data upload                   | `false`          | `...?presigned_url_disabled=true`                                        |
| empty_field_as           | Value used for empty fields when loading CSV data                                                                            | `string`         | `...?empty_field_as=null`                                                |
| location / timezone      | Time zone used when scanning Date/DateTime values (`timezone` is an alias of `location`)                                     | `UTC`            | `...?location=Asia/Shanghai`                                            |
| enable_http_compression  | Enable gzip compression for HTTP requests                                                                                    | `false`          | `...?enable_http_compression=1`                                         |
| enable_otel              | Enable OpenTelemetry HTTP instrumentation                                                                                    | `false`          | `...?enable_otel=true`                                                   |
| debug                    | Enable debug logging                                                                                                         | `false`          | `...?debug=1`                                                            |
| tls_config               | Name of a registered custom TLS config                                                                                       | none             | `...?tls_config=my-tls`                                                  |
