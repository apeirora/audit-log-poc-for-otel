# OpenTelemetry Collector (Audit-Logging)

Let's build our own OTel-collector

## TL;DR

### build

```bash
go install go.opentelemetry.io/collector/cmd/builder@latest
git clone git@github.com:open-telemetry/opentelemetry-collector.git
cd opentelemetry-collector
builder --config=$(realpath ../otel-collector/otelcol-builder.yaml)
```

### run

```bash
./_build/otelcol-audit --config=$(realpath ../otel-collector/config.yaml)
```
