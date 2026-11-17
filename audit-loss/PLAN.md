# Step 1 plan

Folder is `audit-loss/`. Step 2 (the page) is not in this slice. Done when `happy`, `main-unreachable`, and `malformed` match `auditlog.md`.

## Order

1. HMAC and OTLP JSON helpers, with tests.
2. Sink: one file per sequence, fsync, `GET /stats`.
3. Spammer: `POST /run`, `GET /last-run`, faults, threads.
4. Edge: sign in front of the sidecar, verify in front of each collector.
5. Toxic: HTTP proxy, three links, fault API.
6. Collector configs: disk queue, failover connector, `demo.collector`.
7. Compose and `scripts/audit-loss.ps1`.
8. `go test ./...`. Compose when the Docker daemon is up.

## Shape

```text
log-spammer  --127.0.0.1-->  edge sign  -->  sidecar otelcol
                                              disk queue, then failover
                                              /                    \
                                        toxic to-main          toxic to-secondary
                                              |                    |
                                        edge verify            edge verify
                                              |                    |
                                        main otelcol           secondary otelcol
                                              \                    /
                                               toxic to-sink
                                                     |
                                                 audit-sink
```

The sidecar shares the spammer network namespace. The app posts to `127.0.0.1` only.

## Choices

- Collectors are `otelcol-contrib` 0.128.0. `signingprocessor` and `certificatelogverify` are not in that image. `edge` runs in the same container and does HMAC-SHA256 over `run_id`, sequence, emit time, and body. The attribute is `audit.integrity.value`. Verify drops a bad record, counts it on the sink, and still returns 200 so failover does not treat a bad signature as a dead collector. If the collector behind verify is down, verify returns 503 and failover moves the batch.
- The queue is an `otlphttp` exporter with `db_storage` (SQLite, WAL, synchronous FULL) on `/var/lib/otel/queue/queue.db`, aimed at a receiver in the same sidecar. Main and secondary use the same extension for the sink export. That sidecar receiver's only exporter is the failover connector. The exporters under the connector have no queue and no retry, so a failure on main is visible and the batch goes to secondary.
- Inner protocol is OTLP HTTP JSON, compression off, so `toxic` can flip a signature byte.
- Services are Go, standard library only, so they test without Docker.

## Step 1 scenarios in the script

| Script argument | Pass |
| --- | --- |
| `happy` | received == sent, all `main`, no missing |
| `main-unreachable` | blackhole `to-main`: all `secondary`. Clear it, wait 16s, send again: all `main` |
| `malformed` | odd sequences are broken JSON. received > 0, received < sent, rejected > 0 |

The other scenario names stay in `auditlog.md`. Their knobs (faults, delay, stop a container) exist here; the script does not drive them yet.

## Not in this slice

A second app, and building a custom collector with in-process processors.

## Status

Steps 1–7 are in this folder. Step 2 is the `audit-ui` service. Open http://localhost:18083 after compose is up. `go test ./...` covers the page and the pass/fail rules. The Docker daemon was not running when this was written, so the browser was not opened against the stack.

```text
docker compose -f audit-loss/compose.yaml up -d --build
powershell -File audit-loss/scripts/audit-loss.ps1 happy
powershell -File audit-loss/scripts/audit-loss.ps1 main-unreachable
powershell -File audit-loss/scripts/audit-loss.ps1 malformed
```
