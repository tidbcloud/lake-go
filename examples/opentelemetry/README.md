## Lake distributed tracing example

### Prerequisites

1. Setup a TiDB Cloud Lake cluster
2. Setup an OpenTelemetry collector (e.g. Jaeger)
3. Setup lake config toml file with tracing enabled
4. Run the example

### Run

```bash
go run main.go
```

### Result

Totally 10 child spans spawned from parent span, each span contains tracing information from existing lake cluster.
