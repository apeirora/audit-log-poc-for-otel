# audit-loss

Standalone demo of audit log delivery through a sidecar collector, a toxic proxy, main and secondary collectors, and a file sink. It does not use the rest of this repository.

## Requirements

- Docker Desktop, with the daemon running
- Network access on the first build (the collector image downloads `otelcol-contrib` 0.128.0)
- Go 1.25, only if you want to run tests outside Docker

## Start

From the repository root:

```powershell
docker compose -f audit-loss/compose.yaml up -d --build
```

The first build pulls base images and the collector release. When the containers are up, open the UI at <http://localhost:18083>.

| Service | URL |
| --- | --- |
| UI | <http://localhost:18083> |
| Log spammer | <http://localhost:18080> |
| Toxic proxy | <http://localhost:18081> |
| Audit sink | <http://localhost:18082> |
| Sidecar health | <http://localhost:13133> |
| Main collector health | <http://localhost:13134> |
| Secondary collector health | <http://localhost:13135> |

## Scenarios

The script waits until the stack is healthy, sends a short run, and checks the sink.

```powershell
powershell -File audit-loss/scripts/audit-loss.ps1 happy
powershell -File audit-loss/scripts/audit-loss.ps1 main-unreachable
powershell -File audit-loss/scripts/audit-loss.ps1 malformed
```

| Argument | What passes |
| --- | --- |
| `happy` | received equals sent, every record on `main` |
| `main-unreachable` | `to-main` is blackholed, records land on `secondary`, then the fault is cleared and the next run returns to `main` |
| `malformed` | odd sequences are broken JSON: some records arrive, some are rejected |

## Tests without Docker

```powershell
cd audit-loss
go test ./...
```

## Stop

```powershell
docker compose -f audit-loss/compose.yaml down
```

Add `-v` to also delete the queue and sink volumes.
