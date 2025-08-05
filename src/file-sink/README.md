# file-sink

A minimal OpenTelemetry OTLP log receiver for demonstration and testing purposes. This service receives OTLP log data via gRPC and HTTP and writes all received logs to a local file.

## Features

- Receives OTLP logs via gRPC (port 4317)
- Receives OTLP logs via HTTP (port 4318)
- Writes all received log records to `received-logs.txt`
- Minimal dependencies, easy to run and extend

## Usage

### Build

```bash
go build -o file-sink main.go
```

### Run

```bash
./file-sink
```

- The service listens on:
  - gRPC: `0.0.0.0:4317`
  - HTTP: `0.0.0.0:4318`
- All received logs are appended to `received-logs.txt` in the current directory.

### Example OTLP Exporter Configuration

Configure your OpenTelemetry SDK or Collector to send logs to this service:

**gRPC:**

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
```

**HTTP:**

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

## Development

```bash
go run main.go
```

- The log receiver is implemented in Go using the official OpenTelemetry Protobuf definitions and gRPC.
- See `main.go` for details.
