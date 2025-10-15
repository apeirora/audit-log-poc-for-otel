# Guaranteed Delivery Architecture using OpenTelemetry Collector

## Overview

We have implemented a resilient observability pipeline using the OpenTelemetry Collector, designed to guarantee the delivery of telemetry data through disk-backed queues and Kafka. This setup supports both durability and recovery under adverse conditions.

## Stack Architecture

### Ingress Collector

- OpenTelemetry Collector with:
  - Receiver(s) for telemetry input
  - Sending queue with file storage (`filestorage`) for persistent buffering
  - Exporter that writes data into a **Kafka topic**

### Egress Collector (Exporter)

- OpenTelemetry Collector with:
  - Kafka receiver consuming data from the topic
  - Sending queue with file storage, similar to the ingress collector
  - Final exporter to the observability backend (e.g., monitoring system, data lake, etc.)

## Advantages

### Durability / Crash Resilience
- Data queued to disk survives Collector crashes or restarts.
- Ensures in-flight telemetry is not lost due to process failure.

### Graceful Recovery & Backpressure Smoothing
- Persistent queues absorb downstream slowness or outages.
- Prevents upstream data drops and smooths traffic spikes.

### Improved Data Fidelity in Adverse Conditions
- Retains full fidelity even during:
  - Network interruptions
  - Backend throttling
  - Collector restarts or redeployments

## Risks & Trade-Offs

### Volume Attachment Issues (Kubernetes)
- Persistent volume claims (PVCs) can get stuck during:
  - Node failures
  - Preemptive pod evictions
- Manual intervention may be required to reattach volumes.
- Increases recovery time and affects automation.

### Increased Latency (Buffering + Disk Write)
- File-backed queues introduce extra I/O steps:
  - write → read → export
- Adds latency to the ingestion-export path, especially under high load or tight SLA requirements.

### Disk I/O Latency
- Each batch is written to disk (via WAL).
- If `fsync` is enabled (to ensure durable write), performance can be further impacted.
- Default `fsync: false` balances safety and performance but may be unsuitable in all environments.

## Proposed Improvement: On-Demand Persistence Queue

To optimize for both **performance** and **durability**, we propose a dynamic queueing strategy:

- "Only use disk-backed persistent queue when there is backpressure or retry conditions. Otherwise, use fast in-memory queue."

### How it would work

- Normal state: Data flows through an in-memory queue (minimal latency).
- On failure or backpressure:
  - Retryable exporter failures (e.g., 5xx, 429) or
  - Full in-memory queue
  -  fallback to a persistent queue (file storage).
- Recovery: Switch back to memory once conditions normalize.

### Benefits

- Reduces unnecessary disk I/O under healthy conditions
- Maintains resilience and durability only when needed
- Optimizes end-to-end latency and resource usage

## Current Observations

- Currently in **onboarding and ram up** phase
- No significant issues yet:
  - Ingestion rates are moderate
  - No large-scale failures or queue buildup observed
- However, we are proactively identifying scaling challenges
