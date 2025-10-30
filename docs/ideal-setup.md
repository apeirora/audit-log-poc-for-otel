# Guidance Document: Reliable Audit Logging with OpenTelemetry

This document describes the recommended 3-tier architecture (Client SDK → OpenTelemetry Collector → Final Storage Sink) for highly reliable
audit log delivery. It focuses on minimizing data loss, ensuring long retention, and keeping operational complexity under control.

## Scope & Goals

Primary goals:

- No audit event loss ("at least once" delivery – duplicates acceptable, loss is not).
- Clear separation of audit logs from regular application telemetry (logs/traces/metrics) to avoid resource contention.
- Predictable retention and compliance (e.g. 10+ years depending on regulation).
- Operable at scale with simple failure recovery.

Non-goals:

- Ultra low latency for audit logs (latency is secondary to durability).
- Mixing audit and high-volume debug/info logs in the same pipeline.

## Architecture Overview (3 Tiers)

1. Client SDK Tier: Produces audit log records and sends them directly to a dedicated Audit OpenTelemetry (OTel) Collector endpoint.
2. Collector Tier: Persists and forwards audit logs using durable queues to the Final Storage Sink. Acts only as a buffering and routing
   layer.
3. Final Storage Sink Tier: Long-term storage (e.g. SIEM, Data Lake, search cluster) with backups and legal retention controls.

Rationale: Separating tiers isolates failure domains (client, transport, storage) and allows independent scaling and policy enforcement.

## 1. Client SDK Tier Guidelines

Use a dedicated logger / pipeline for audit logs distinct from your regular application logging/tracing pipeline.

Recommended components:

- [LoggerProvider][LoggerProvider] with a Simple (non-batching) [LogRecordProcessor][LogRecordProcessor].
- A custom [AuditLogRecordProcessor][AuditLogRecordProcessor] ensuring durability (e.g. persistent local queue, no dropping when queue is
  full).
- OTLP (OpenTelemetry Protocol) [LogRecordExporter][LogRecordExporter] configured with retry and a dedicated endpoint
  ([`setEndpoint()`](https://opentelemetry.io/docs/languages/java/sdk/#opentelemetrysdk)) pointing to the audit collector.

Why no batching at client side for audit logs?:

- Batching trades reliability for throughput and can increase loss risk during crashes.

Key settings:

- Enable persistent local buffering (e.g. file-based) where available.
- Use exponential backoff retries with upper bound.
- Prefer gRPC OTLP export for efficiency; HTTP may be used for constrained environments.

Failure considerations:

- If the network is down, client-side persistence must retain events until restored.
- If local disk fills, trigger alerts; do not silently discard audit entries.

## 2. Collector Tier Guidelines

Run a dedicated OTel Collector instance (or set of instances) for audit logs – do not share with high-volume telemetry.

Principles:

- Use persistent sending queue ([export helper v2][batchv2]) – never the [deprecated batch processor][batchv1] for critical audit paths.
- Only add batching if load metrics prove necessary (opt-in, not default).
- Treat the collector storage as transient, not authoritative.

[Configuration](https://opentelemetry.io/docs/collector/configuration/) Example (`config.yaml`):

```yaml
extensions:
  # See: https://opentelemetry.io/docs/collector/configuration/#extensions
  file_storage:
    directory: /var/lib/otelcol/storage
    create_directory: true
  health_check:
    endpoint: ${env:MY_POD_IP}:13133

receivers:
  # See: https://opentelemetry.io/docs/collector/configuration/#receivers
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors: {} # See: https://opentelemetry.io/docs/collector/configuration/#processors

exporters:
  # See: https://opentelemetry.io/docs/collector/configuration/#exporters
  otlp:
    endpoint: log-sink:4317
    # See: https://github.com/open-telemetry/opentelemetry-collector/issues/8122
    sending_queue:
      enabled: true
      storage: file_storage
    retry_on_failure:
      enabled: true

service:
  # See: https://opentelemetry.io/docs/collector/configuration/#service
  extensions: [file_storage, health_check]
  pipelines:
    logs:
      receivers: [otlp]
      processors: []
      exporters: [otlp]
```

Operational Notes:

- Monitor queue depth; only decommission a node when its persistent queue is empty.
- Health check endpoint must be scraped; failing health triggers remediation.
- Use node-local filesystem (cluster node persistent path) to minimize latency; weigh trade-offs vs. network-attached volumes.

## 3. Final Storage Sink Tier Guidelines

Examples: SIEM (Security Information and Event Management), Data Lake (e.g. S3/GCS/HDFS), OpenSearch, Elasticsearch.

Requirements:

- Persistent Volumes (PV) with regular backups and tested restore workflows.
- WORM (Write Once Read Many) or immutability features where regulation requires.
- Indexing strategy permitting multi-year retention (tiered storage, cold archive, glacier-like deep storage).
- Access controls & audit trails on read operations.
- Encryption at rest.

Do not rely on the Collector for long-term retention; it is transient (unless configured as recommended above)!

## Reliability & Delivery Semantics

Desired delivery: At least once.

- In case of unavailability of sinks, prefer duplicates over loss.
- Duplicate detection (idempotency) can be handled downstream using event IDs.
- Include a stable unique identifier in each audit log to allow for downstream de-duplication.

Loss Prevention Layers:

1. Client: local persistence (disk queue) – crash resiliency.
2. Collector: persistent sending queue – network / downstream outage buffering.
3. Final Sink: durability (redundant storage (e.g. RAID), backups).

## Monitoring & Alerting

Monitoring of the involved components of the data delivery stack is critical, as it will unveil upcoming threats of data loss early and can
be used to trigger remediation actions before data loss occurs. In a distributed system, where delivery can never be 100% guaranteed,
monitoring is crucial to get at least close to 100%.

Track and alert on:

- Client queue size & age (oldest event timestamp).
- Collector sending_queue depth and retry counts, failed requests counts.
- Export latency (p50, p95) vs. SLOs.
- Final sink ingestion lag (difference between event time and indexed time).
- Storage capacity thresholds and projections (time to full).

Set thresholds for proactive scaling:

- If queue age > defined SLA (e.g. 5 min), investigate network/backpressure.
- If retry rate spikes, check sink health.
- If storage capacity exceeds threshold, take care of storage extension and/or investigate network/backpressure.

## Scaling Strategy

Horizontal scaling points:

- Add more Collector instances behind DNS / load balancer for increased ingestion throughput.
- Partition audit logs by tenant or domain if cardinality grows and spread out to tenant or domain-specific Collectors.

Client side remains lightweight due to no batching – CPU overhead minimal.

## Security & Compliance

- Encrypt in transit (TLS for OTLP gRPC/HTTP). Mutual TLS for sensitive environments.
- Restrict Collector endpoints with network policy (only allow known client subnets).
- Sign or hash audit events (optional) for tamper detection before storage.
- Maintain immutable backups; test restore quarterly.

PII Handling:

- Classify fields; avoid storing unnecessary personal data.
- Apply tokenization or pseudonymization where feasible before export.
- If necessary, encrypt sensitive fields at the application level.

## Failure Modes & Mitigations

| Failure Mode                 | Mitigation                                                           |
| ---------------------------- | -------------------------------------------------------------------- |
| Client crash                 | Retrieve unsent Audit Logs from local disk queue + resend to sink    |
| Network outage               | Client queue grows; alert on age; Collector persistent queue buffers |
| Collector restart            | file_storage + sending_queue preserves state                         |
| Final sink outage            | Collector retry + queue; monitor depth                               |
| Disk full (client/collector) | Alerts; autoscale storage; pause intake if critical                  |
| Data corruption              | Checksums / hashing; sink replication; backup restore                |

## Implementation Checklist

Client SDK:

- [ ] Dedicated Audit [LoggerProvider][LoggerProvider]
- [ ] [Simple](https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/logs/sdk.md#simple-processor)
      (non-batching) or [AuditLogRecordProcessor][AuditLogRecordProcessor]
- [ ] [Persistent][AuditLogRecordProcessor] local queue enabled
- [ ] OTLP exporter with [retry + endpoint isolation][AuditLogRecordProcessor]

Collector:

- [ ] Dedicated deployment (not shared with high-volume telemetry)
- [ ] [`sending_queue`](https://pkg.go.dev/go.opentelemetry.io/collector/exporter/exporterhelper#readme-persistent-queue) enabled with
      `file_storage`
- [ ] [Health check](https://pkg.go.dev/github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension#readme-health-check)
      monitored
- [ ] [Queue depth metric](https://opentelemetry.io/docs/collector/internal-telemetry/#basic-level-metrics) alerts configured

Final Sink:

- [ ] Retention policy documented
- [ ] Backups + restore runbook
- [ ] Access control & audit of reads
- [ ] Capacity & cost monitoring

Cross-Cutting:

- [ ] TLS enabled end-to-end
- [ ] Unique event IDs used inside audit log events
- [ ] Monitoring dashboards created (queues, latency, errors)
- [ ] Runbook for each failure mode

## Glossary & References

Acronyms / Terms:

- OTel / OpenTelemetry: Open-source observability framework (<https://opentelemetry.io/>)
- OTLP: OpenTelemetry Protocol (<https://github.com/open-telemetry/opentelemetry-specification/tree/main/specification/protocol>)
- gRPC: High-performance RPC framework (<https://grpc.io/>)
- SIEM: Security Information and Event Management.
- PV: Persistent Volume (Kubernetes storage abstraction).
- SLO: Service Level Objective.
- WORM: Write Once Read Many storage model.
- TLS: Transport Layer Security.
- PII: Personally Identifiable Information.

Referenced Issues / PRs:

- Export helper with persistent queue (Batch v2 discussion): [#8122][batchv2]
- Proposed AuditLogRecordProcessor (Java PR): [AuditLogRecordProcessor]
- Collector scaling guidance: [Scaling the Collector][scaling]

[AuditLogRecordProcessor]: https://github.com/apeirora/opentelemetry-java/pull/2
[batchv1]: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/logs/sdk.md#batching-processor
[batchv2]: https://github.com/open-telemetry/opentelemetry-collector/issues/8122
[LoggerProvider]: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/logs/sdk.md#loggerprovider
[LogRecordExporter]: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/logs/sdk.md#logrecordexporter
[LogRecordProcessor]: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/logs/sdk.md#logrecordprocessor
[scaling]: https://opentelemetry.io/docs/collector/scaling/
