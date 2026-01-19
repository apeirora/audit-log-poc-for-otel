# Log pipeline metrics

## Exporter send attempts (per exporter)
- `otelcol_exporter_sent_log_records`
- `otelcol_exporter_send_failed_log_records`
- Counted per send attempt; any non-nil error marks the whole batch as failed. Retries add more increments.

## Queue instrumentation (when `sending_queue` is enabled)
- Gauges: queue size and queue capacity (in batches).
- Counters: enqueue failures for logs.
- Histograms: batch size (items) and batch size (bytes) recorded at enqueue time.

## Consumed-side counters (behind feature gate)
- Item and byte counts as data leaves processors toward exporters, labeled per component/exporter.
- Requires `telemetry.newPipelineTelemetry` to be enabled; size counter depends on telemetry level.

## Exporter helper chain
- `BaseExporter` composes: timeout → retry → obsreport (span + sent/failed counters) → optional queue wrapper → actual exporter.
- Counts are taken before downstream mutation so batching processors do not change the numerator.
- Spans are started per export call with attributes: exporter name and `data_type=logs`.

## Limitations and caveats
- Attempt-based counting means retries inflate sent/failed totals; not unique record counts.
- No partial success visibility: any error marks the entire batch failed.
- Queue metrics are batch-based; histograms emit only when queueing is enabled.
- Profiles signal is not instrumented by obsreport/queue.
- Size counters rely on telemetry level/gate; if disabled you see only item counts.

## What to watch
- Delivery health: `otelcol_exporter_send_failed_log_records` vs `otelcol_exporter_sent_log_records`.
- Backpressure: `otelcol_exporter_queue_size` vs `queue_capacity`; watch enqueue-failure counter for drops.
- Batch shape: `otelcol_exporter_queue_batch_send_size` and `otelcol_exporter_queue_batch_send_size_bytes`.
- Pipeline loss/refusal (with gate on): consumed item/size counters plus refusal/failure attributes from obsconsumer wrappers.