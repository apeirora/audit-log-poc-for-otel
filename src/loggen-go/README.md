# loggen-go

`loggen-go` is a simple log generator written in Go, designed to emit logs using [OpenTelemetry](https://opentelemetry.io/) (OTel) for
testing and demonstration purposes. It is part of the [audit-log-poc-for-otel](https://github.com/apeirora/audit-log-poc-for-otel) project,
which explores audit logging scenarios with OTel.

## Overview

This tool emits a fixed number of logs to an OTel-compatible backend using the OTLP protocol. It is useful for testing log pipelines,
collectors, and observability setups.

## Build

### With Go

```bash
go build -o loggen-go main.go
```

### With Docker

```bash
docker build -t loggen-go .
```

## Run

By default, `loggen-go` will attempt to send logs to the default OTLP endpoint (`localhost:4317`). You may need to configure your OTel
Collector or backend accordingly.

```bash
./loggen-go
```

## Configuration

`loggen-go` uses the default OTel environment variables for configuration. You can set the OTLP endpoint and other options as follows:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_EXPORTER_OTLP_INSECURE=true
./loggen-go
```

## Example Output

The tool emits 10 log records with a simple message and a `log-count` attribute. Example log record:

```bash
{
  "severity": "INFO",
  "body": "test",
  "attributes": {
    "log-count": "1"
  }
}
```

## Purpose

This tool is intended for development, testing, and demonstration of OTel log ingestion and processing. It is not intended for production
use.
