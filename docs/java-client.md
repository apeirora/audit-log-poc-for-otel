# Robustness Improvements for OpenTelemetry Java Client

<!-- markdownlint-disable MD024 -->

- [Robustness Improvements for OpenTelemetry Java Client](#robustness-improvements-for-opentelemetry-java-client)
  - [Overview](#overview)
  - [Problem Statement](#problem-statement)
    - [Current Situation](#current-situation)
    - [Impact](#impact)
  - [Solution: Error Consumer Pattern](#solution-error-consumer-pattern)
    - [Concept](#concept)
    - [Technical Implementation](#technical-implementation)
      - [1. ExportErrorContext](#1-exporterrorcontext)
      - [2. BatchLogRecordProcessor Enhancement](#2-batchlogrecordprocessor-enhancement)
      - [3. SimpleLogRecordProcessor Enhancement](#3-simplelogrecordprocessor-enhancement)
  - [Usage Examples](#usage-examples)
    - [1. Global Error Handler](#1-global-error-handler)
    - [2. Context-Specific Error Handling](#2-context-specific-error-handling)
    - [3. Integration with Monitoring Systems](#3-integration-with-monitoring-systems)
  - [Benefits](#benefits)
    - [1. Better Observability](#1-better-observability)
    - [2. Increased Robustness](#2-increased-robustness)
    - [3. Flexibility](#3-flexibility)
    - [4. Compliance and Audit](#4-compliance-and-audit)
  - [Implementation Details](#implementation-details)
    - [Phase 1: Core Implementation](#phase-1-core-implementation)
    - [Phase 2: Testing and Validation](#phase-2-testing-and-validation)
    - [Phase 3: Extensions](#phase-3-extensions)
  - [Migration and Backward Compatibility](#migration-and-backward-compatibility)
    - [Backward Compatibility](#backward-compatibility)
    - [Migration Guidelines](#migration-guidelines)
  - [Performance Considerations](#performance-considerations)
    - [Overhead](#overhead)
    - [Best Practices](#best-practices)
  - [Alternative Approaches](#alternative-approaches)
    - [1. Event-Based Error Handling](#1-event-based-error-handling)
      - [Concept](#concept-1)
    - [2. Dedicated Fallback Exporter](#2-dedicated-fallback-exporter)
      - [Concept](#concept-2)
    - [3. Retry Mechanism with Exponential Backoff](#3-retry-mechanism-with-exponential-backoff)
      - [Concept](#concept-3)
    - [4. Circuit Breaker Pattern](#4-circuit-breaker-pattern)
      - [Concept](#concept-4)
    - [5. Dead Letter Queue Pattern](#5-dead-letter-queue-pattern)
      - [Concept](#concept-5)
    - [6. Metrics-Based Monitoring Approach](#6-metrics-based-monitoring-approach)
      - [Concept](#concept-6)
  - [Hybrid Approaches](#hybrid-approaches)
    - [Recommended Combination for Enterprise Environments](#recommended-combination-for-enterprise-environments)
  - [Conclusion](#conclusion)

## Overview

This document describes planned improvements to make the OpenTelemetry Java Client more robust. The main goal is to give developers better
control and ways to react to export errors, so that audit log messages are not lost.

## Problem Statement

### Current Situation

- When log record exports fail, errors are only logged internally
- Applications cannot react to failed exports
- Critical log messages (like audit logs) can be lost without the app knowing
- No standard error handling for different export scenarios

### Impact

- Loss of important audit log data without notification
- Hard to diagnose export problems
- No fallback or retry mechanisms
- Limited observability of the log pipeline itself

## Solution: Error Consumer Pattern

### Concept

Implement an **Error Consumer Pattern** that lets developers:

- Define custom error handling logic for failed log exports
- React to export errors at batch or individual record level
- Integrate flexibly with monitoring and alerting systems

### Technical Implementation

Sketched out in [this pull request](https://github.com/hilmarf/opentelemetry-java/pull/1).

#### 1. ExportErrorContext

```java
@Immutable
public class ExportErrorContext {
  public static final ContextKey<Consumer<Collection<LogRecordData>>> KEY =
      ContextKey.named("export-error-consumer");
}
```

#### 2. BatchLogRecordProcessor Enhancement

- New `setErrorConsumer()` method in the builder
- Error consumer is called on export errors
- Supports batch-level and queue-overflow error handling

#### 3. SimpleLogRecordProcessor Enhancement

- Integrates the error consumer pattern
- Consistent error handling across processor types

## Usage Examples

### 1. Global Error Handler

```java
// Custom error handler for all failed exports
Consumer<Collection<LogRecordData>> globalErrorHandler = failedRecords -> {
    failedRecords.forEach(record -> {
        slf4jLogger.error("Failed to export critical log: {}", record.getBodyValue());
        // More actions: alerting, fallback storage, etc.
    });
};

// BatchLogRecordProcessor with error handler
LogRecordProcessor processor = BatchLogRecordProcessor.builder(exporter)
    .setErrorConsumer(globalErrorHandler)
    .build();
```

### 2. Context-Specific Error Handling

```java
// Custom error handling per log message
Context errorContext = Context.current().with(ExportErrorContext.KEY,
    failedRecords -> {
        // Special handling for critical business logs
        handleCriticalLogFailure(failedRecords);
    });

logger.logRecordBuilder()
    .setBody("Critical business transaction completed")
    .setContext(errorContext)
    .emit();
```

### 3. Integration with Monitoring Systems

```java
Consumer<Collection<LogRecordData>> monitoringIntegration = failedRecords -> {
    // Metrics for failed exports
    failureCounter.increment(failedRecords.size());

    // Alert for critical errors
    if (containsCriticalLogs(failedRecords)) {
        alertingService.sendAlert("Critical log export failed");
    }

    // Fallback to local storage
    localStorageService.store(failedRecords);
};
```

## Benefits

### 1. Better Observability

- Full transparency on export success and errors
- Integration with existing monitoring
- Detailed metrics for telemetry pipeline performance

### 2. Increased Robustness

- App-specific error handling
- Retry mechanisms possible
- Fallback strategies for critical data

### 3. Flexibility

- Context-dependent error handling
- Different error handlers for different log types
- Easy integration, no breaking changes

### 4. Compliance and Audit

- Ensures critical audit logs are not lost
- Traceable error handling
- Meets regulatory requirements

## Implementation Details

### Phase 1: Core Implementation

- [✅] `ExportErrorContext` class
- [✅] `BatchLogRecordProcessor` error consumer integration
- [✅] `SimpleLogRecordProcessor` error consumer integration
- [✅] Builder pattern updates

### Phase 2: Testing and Validation

- [ ] Unit tests
- [ ] Integration tests
- [ ] Performance assessment
- [ ] Documentation and examples

### Phase 3: Extensions

- [ ] Trace and metrics support
- [ ] Standard error handler library

## Migration and Backward Compatibility

### Backward Compatibility

- Fully backward compatible
- Default behavior unchanged
- New functionality is opt-in

### Migration Guidelines

```java
// Existing code (unchanged)
BatchLogRecordProcessor processor = BatchLogRecordProcessor.builder(exporter).build();

// Enhanced code (optional)
BatchLogRecordProcessor processor = BatchLogRecordProcessor.builder(exporter)
    .setErrorConsumer(myErrorHandler)
    .build();
```

## Performance Considerations

### Overhead

- Minimal performance overhead, only on errors
- No impact on happy path
- Async error handling possible

### Best Practices

- Error handlers should be non-blocking
- Avoid recursive export attempts in error handler
- Rate limiting for alert mechanisms

## Alternative Approaches

Besides the error consumer pattern, there are other ways to improve robustness:

### 1. Event-Based Error Handling

#### Concept

Event-based error handling uses an event bus to publish export errors as events. Instead of handling errors directly in the log processor,
errors are sent as events to a central event bus. Apps can register event handlers to react to these errors. This decouples error detection
from error handling and makes integration easier.

- Errors are published as structured events (e.g. `LogExportFailedEvent`)
- Multiple event handlers can react in parallel (alerting, fallback, monitoring)
- Error handling can be changed at runtime
- Integration with event bus frameworks (e.g. Guava EventBus, Spring Events)

Example:

```java
// Event-based approach
public class LogExportFailedEvent {
    private final Collection<LogRecordData> failedRecords;
    private final Throwable cause;
    private final String exporterName;
}

// Event publisher in processor
eventBus.publish(new LogExportFailedEvent(failedRecords, exception, exporterName));

// Event subscriber in app
@EventHandler
public void handleLogExportFailure(LogExportFailedEvent event) {
    // Custom error handling
}
```

**Pros:**

- Decouples processor and error handling
- Multiple event handlers possible
- Standard event structures

**Cons:**

- Needs event bus framework
- More complex configuration
- Possible performance overhead

### 2. Dedicated Fallback Exporter

#### Concept

Implement a fallback mechanism at exporter level:

```java
public class FallbackLogRecordExporter implements LogRecordExporter {
    private final LogRecordExporter primaryExporter;
    private final LogRecordExporter fallbackExporter;

    @Override
    public CompletableResultCode export(Collection<LogRecordData> logs) {
        CompletableResultCode result = primaryExporter.export(logs);

        return result.whenComplete(() -> {
            if (!result.isSuccess()) {
                // Fallback to secondary exporter
                fallbackExporter.export(logs);
            }
        });
    }
}
```

**Pros:**

- Automatic fallback logic
- Transparent for processor
- Easy configuration

**Cons:**

- Double export overhead on errors
- Limited flexibility
- No context-specific handling

### 3. Retry Mechanism with Exponential Backoff

#### Concept

Built-in retry logic in processors:

```java
public class RetryableLogRecordProcessor implements LogRecordProcessor {
    private final RetryPolicy retryPolicy;

    private void exportWithRetry(Collection<LogRecordData> logs) {
        int attempts = 0;
        while (attempts < retryPolicy.getMaxAttempts()) {
            try {
                CompletableResultCode result = exporter.export(logs);
                if (result.isSuccess()) return;

                Thread.sleep(retryPolicy.getBackoffMs(attempts));
                attempts++;
            } catch (Exception e) {
                if (attempts == retryPolicy.getMaxAttempts() - 1) {
                    // Final failure handling
                    handleFinalFailure(logs, e);
                }
            }
        }
    }
}
```

**Pros:**

- Automatic retry logic
- Configurable strategies
- Reduces transient errors

**Cons:**

- Higher latency on errors
- More complex configuration
- Possible blocking on persistent errors

### 4. Circuit Breaker Pattern

#### Concept

Implement a circuit breaker for export operations:

```java
public class CircuitBreakerLogRecordExporter implements LogRecordExporter {
    private final CircuitBreaker circuitBreaker;
    private final LogRecordExporter delegateExporter;

    @Override
    public CompletableResultCode export(Collection<LogRecordData> logs) {
        if (circuitBreaker.getState() == CircuitBreaker.State.OPEN) {
            // Circuit is open - fallback directly
            return handleCircuitOpen(logs);
        }

        try {
            CompletableResultCode result = delegateExporter.export(logs);
            circuitBreaker.recordSuccess();
            return result;
        } catch (Exception e) {
            circuitBreaker.recordFailure();
            throw e;
        }
    }
}
```

**Pros:**

- Prevents overload of failing services
- Automatic recovery
- Protects against cascade failures

**Cons:**

- Complex configuration
- Extra infrastructure
- Loss of logs during "open" phase

### 5. Dead Letter Queue Pattern

#### Concept

Use a dead letter queue for failed exports:

```java
public class DeadLetterQueueProcessor implements LogRecordProcessor {
    private final Queue<LogRecordData> deadLetterQueue;
    private final ScheduledExecutorService retryScheduler;

    private void handleExportFailure(Collection<LogRecordData> failedLogs) {
        // Add to dead letter queue
        deadLetterQueue.addAll(failedLogs);

        // Periodic retry attempts
        scheduleRetryAttempt();
    }

    private void scheduleRetryAttempt() {
        retryScheduler.schedule(() -> {
            List<LogRecordData> batch = new ArrayList<>();
            while (!deadLetterQueue.isEmpty() && batch.size() < BATCH_SIZE) {
                batch.add(deadLetterQueue.poll());
            }

            if (!batch.isEmpty()) {
                attemptReexport(batch);
            }
        }, RETRY_DELAY, TimeUnit.SECONDS);
    }
}
```

**Pros:**

- No logs lost
- Async retry
- Decouples from main process

**Cons:**

- Extra memory overhead
- Complex queue management
- Possible unlimited queue size

### 6. Metrics-Based Monitoring Approach

#### Concept

Focus on metrics instead of error handling:

```java
public class MetricsEnhancedProcessor implements LogRecordProcessor {
    private final Counter exportSuccessCounter;
    private final Counter exportFailureCounter;
    private final Timer exportLatency;
    private final Gauge queueSize;

    @Override
    public void onEmit(Context context, ReadWriteLogRecord logRecord) {
        Timer.Sample sample = Timer.start();

        try {
            CompletableResultCode result = exporter.export(logs);
            sample.stop(exportLatency);

            if (result.isSuccess()) {
                exportSuccessCounter.increment();
            } else {
                exportFailureCounter.increment();
                // Alert based on metrics threshold
                checkFailureThreshold();
            }
        } catch (Exception e) {
            exportFailureCounter.increment();
            sample.stop(exportLatency);
        }
    }
}
```

**Pros:**

- Full observability
- Integration with monitoring systems
- Proactive alerting

**Cons:**

- No direct error handling
- Depends on external monitoring
- Logs can still be lost

## Hybrid Approaches

You can also combine patterns:

### Recommended Combination for Enterprise Environments

```java
// Combine Error Consumer + Circuit Breaker + Metrics
LogRecordProcessor processor = BatchLogRecordProcessor.builder(
    CircuitBreakerLogRecordExporter.wrap(
        MetricsEnhancedExporter.wrap(primaryExporter)
    ))
    .setErrorConsumer(auditLogFailureHandler)
    .build();
```

This combination offers:

- Error consumer for audit log handling
- Circuit breaker for system protection
- Metrics for monitoring and alerting

## Conclusion

The error consumer pattern implementation makes the OpenTelemetry Java Client much more robust and observable. Developers can react to
export problems and protect critical log data, without hurting performance or compatibility.

The implementation follows established patterns and integrates smoothly into OpenTelemetry, while opening new possibilities for
enterprise-grade audit log
