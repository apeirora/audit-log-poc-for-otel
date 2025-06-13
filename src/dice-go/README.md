# REST Logging App with OpenTelemetry

Dummy rest application that logs requests using OpenTelemetry.

## Start

```bash
eval "$(task otel:export-collector --silent)"
cd src/dice-go
go mod tidy
go run main.go
```

## Test

```bash
curl http://localhost:8081/rolldice/${USER}
```
