# Robustness Improvements for OpenTelemetry Go Client

This document describes planned improvements to make the OpenTelemetry Go Client more robust. The main goal is to give developers better
control and ways to react to export errors, so that audit log messages are not lost.

- [Robustness Improvements for OpenTelemetry Go Client](#robustness-improvements-for-opentelemetry-go-client)
  - [Problem Statement](#problem-statement)
    - [Current Situation](#current-situation)
    - [Impact](#impact)
  - [Solution: Error Handler Pattern](#solution-error-handler-pattern)
    - [Concept](#concept)
    - [Technical Implementation](#technical-implementation)
      - [1. ExportErrorHandler Interface](#1-exporterrorhandler-interface)
      - [2. BatchProcessor Enhancement](#2-batchprocessor-enhancement)
      - [3. SimpleProcessor Enhancement](#3-simpleprocessor-enhancement)
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
    - [1. Channel-Based Error Handling](#1-channel-based-error-handling)
    - [2. Middleware Pattern with Exporters](#2-middleware-pattern-with-exporters)
    - [3. Circuit Breaker Pattern](#3-circuit-breaker-pattern)
    - [4. Retry Mechanism with Exponential Backoff](#4-retry-mechanism-with-exponential-backoff)
    - [5. Dead Letter Queue Pattern](#5-dead-letter-queue-pattern)
    - [6. Metrics-Based Monitoring Approach](#6-metrics-based-monitoring-approach)
  - [Hybrid Approaches](#hybrid-approaches)
    - [Recommended Combination for Enterprise Environments](#recommended-combination-for-enterprise-environments)
  - [Conclusion](#conclusion)

## Problem Statement

### Current Situation

- When log record exports fail, errors are only handled by the global `otel.Handle(err)` function
- Applications cannot react to specific failed exports
- Critical log messages (like audit logs) can be lost without the app knowing
- No standard error handling for different export scenarios
- Limited visibility into export pipeline failures

### Impact

- Loss of important audit log data without notification
- Hard to diagnose export problems in production
- No fallback or retry mechanisms for critical data
- Limited observability of the log pipeline itself
- Compliance issues when audit logs are silently lost

## Solution: Error Handler Pattern

### Concept

Implement an **Error Handler Pattern** that lets developers:

- Define custom error handling logic for failed log exports
- React to export errors at batch or individual record level
- Integrate flexibly with monitoring and alerting systems
- Maintain Go's idiomatic error handling patterns

### Technical Implementation

#### 1. ExportErrorHandler Interface

```go
// ExportErrorHandler handles errors that occur during log record export.
type ExportErrorHandler interface {
 // HandleExportError is called when an export operation fails.
 // records contains the log records that failed to export.
 // err is the error that caused the export to fail.
 HandleExportError(ctx context.Context, records []Record, err error)
}

// ExportErrorHandlerFunc is a convenience adapter to allow the use of a function
// as an ExportErrorHandler.
type ExportErrorHandlerFunc func(ctx context.Context, records []Record, err error)

var _ ExportErrorHandler = ExportErrorHandlerFunc(nil)

// HandleExportError handles the export error by calling the ExportErrorHandlerFunc itself.
func (f ExportErrorHandlerFunc) HandleExportError(ctx context.Context, records []Record, err error) {
 f(ctx, records, err)
}

// ExportErrorContext provides context for export errors including metadata.
type ExportErrorContext struct {
 ProcessorType  string                 // "batch" or "simple"
 ExporterName   string                 // Name/type of the exporter
 RetryAttempt   int                    // Current retry attempt (if applicable)
 Metadata       map[string]interface{} // Additional context data
}

// EnhancedExportErrorHandler provides more detailed error context.
type EnhancedExportErrorHandler interface {
 // HandleExportErrorWithContext is called when an export operation fails with additional context.
 HandleExportErrorWithContext(ctx context.Context, records []Record, err error, errorCtx ExportErrorContext)
}
```

#### 2. BatchProcessor Enhancement

```go
// BatchProcessorOption applies a configuration to a BatchProcessor.
type BatchProcessorOption interface {
 apply(batchConfig) batchConfig
}

// WithExportErrorHandler configures the BatchProcessor to use the provided
// error handler for export failures.
func WithExportErrorHandler(handler ExportErrorHandler) BatchProcessorOption {
 return batchProcessorOptionFunc(func(cfg batchConfig) batchConfig {
  cfg.exportErrorHandler = handler
  return cfg
 })
}

// Enhanced BatchProcessor struct
type BatchProcessor struct {
 exporter           Exporter
 exportErrorHandler ExportErrorHandler
 // ... other existing fields
}

// Modified export method in BatchProcessor
func (b *BatchProcessor) export(ctx context.Context, records []Record) {
 if err := b.exporter.Export(ctx, records); err != nil {
  if b.exportErrorHandler != nil {
   // Handle error asynchronously to avoid blocking the main export flow
   go b.exportErrorHandler.HandleExportError(ctx, records, err)
  }
  // Still report to global error handler for backward compatibility
  otel.Handle(err)
 }
}
```

#### 3. SimpleProcessor Enhancement

```go
// SimpleProcessorOption applies a configuration to a SimpleProcessor.
type SimpleProcessorOption interface {
 apply()
}

// WithExportErrorHandler configures the SimpleProcessor to use the provided
// error handler for export failures.
func WithExportErrorHandler(handler ExportErrorHandler) SimpleProcessorOption {
 return simpleProcessorOptionFunc(func(cfg *simpleConfig) {
  cfg.exportErrorHandler = handler
 })
}

// Enhanced SimpleProcessor
type SimpleProcessor struct {
 exporter           Exporter
 exportErrorHandler ExportErrorHandler
 mu                 sync.Mutex
}

// Modified OnEmit method
func (s *SimpleProcessor) OnEmit(ctx context.Context, r *Record) error {
 if s.exporter == nil {
  return nil
 }

 s.mu.Lock()
 defer s.mu.Unlock()

 records := simpleProcRecordsPool.Get().(*[]Record)
 (*records)[0] = *r
 defer func() {
  simpleProcRecordsPool.Put(records)
 }()

 if err := s.exporter.Export(ctx, *records); err != nil {
  if s.exportErrorHandler != nil {
   s.exportErrorHandler.HandleExportError(ctx, *records, err)
  }
  return err // Return error for caller to handle
 }
 return nil
}
```

## Usage Examples

### 1. Global Error Handler

```go
// Custom error handler for all failed exports
globalErrorHandler := log.ExportErrorHandlerFunc(func(ctx context.Context, records []log.Record, err error) {
 for _, record := range records {
  slog.Error("Failed to export critical log",
   "error", err,
   "body", record.Body(),
   "timestamp", record.Timestamp())

  // Additional actions: alerting, fallback storage, etc.
 }
})

// BatchProcessor with error handler
processor := log.NewBatchProcessor(exporter,
 log.WithExportErrorHandler(globalErrorHandler),
)

// Create LoggerProvider with the processor
provider := log.NewLoggerProvider(
 log.WithProcessor(processor),
)
```

### 2. Context-Specific Error Handling

```go
// Context key for record-specific error handling
type errorHandlerKey struct{}

// Set context-specific error handler
ctx := context.WithValue(context.Background(), errorHandlerKey{},
 log.ExportErrorHandlerFunc(func(ctx context.Context, records []log.Record, err error) {
  // Special handling for critical business logs
  handleCriticalLogFailure(records, err)
 }))

// Use the context when emitting logs
logger.InfoContext(ctx, "Critical business transaction completed",
 "transaction_id", "12345",
 "amount", 1000.00)
```

### 3. Integration with Monitoring Systems

```go
// Prometheus metrics for monitoring
var (
 exportFailures = prometheus.NewCounterVec(
  prometheus.CounterOpts{
   Name: "otel_log_export_failures_total",
   Help: "Total number of log export failures",
  },
  []string{"exporter_type", "error_type"},
 )
)

monitoringHandler := log.ExportErrorHandlerFunc(func(ctx context.Context, records []log.Record, err error) {
 // Increment failure metrics
 exportFailures.WithLabelValues("otlp", classifyError(err)).Inc()

 // Alert for critical errors
 if containsCriticalLogs(records) {
  alertingService.SendAlert("Critical log export failed", err)
 }

 // Fallback to local storage
 localStorageService.Store(records)
})
```

## Benefits

### 1. Better Observability

- Full transparency on export success and errors
- Integration with existing monitoring systems
- Detailed metrics for telemetry pipeline performance
- Structured error reporting with context

### 2. Increased Robustness

- Application-specific error handling
- Retry mechanisms possible through custom handlers
- Fallback strategies for critical data
- Graceful degradation patterns

### 3. Flexibility

- Context-dependent error handling
- Different error handlers for different log types
- Easy integration, no breaking changes
- Idiomatic Go patterns

### 4. Compliance and Audit

- Ensures critical audit logs are not lost
- Traceable error handling with structured logging
- Meets regulatory requirements
- Audit trail for failed exports

## Implementation Details

### Phase 1: Core Implementation

- [ ] `ExportErrorHandler` interface definition
- [ ] `BatchProcessor` error handler integration
- [ ] `SimpleProcessor` error handler integration
- [ ] Builder pattern updates with `WithExportErrorHandler` options

### Phase 2: Testing and Validation

- [ ] Unit tests for error handling scenarios
- [ ] Integration tests with real exporters
- [ ] Performance benchmarks
- [ ] Documentation and examples

### Phase 3: Extensions

- [ ] Enhanced error context with metadata
- [ ] Trace and metrics processor support
- [ ] Standard error handler library
- [ ] Context-aware error handling

## Migration and Backward Compatibility

### Backward Compatibility

- Fully backward compatible
- Default behavior unchanged (still uses `otel.Handle`)
- New functionality is opt-in
- Existing processors work without modifications

### Migration Guidelines

```go
// Existing code (unchanged)
processor := log.NewBatchProcessor(exporter)

// Enhanced code (optional)
processor := log.NewBatchProcessor(exporter,
 log.WithExportErrorHandler(myErrorHandler),
)
```

## Performance Considerations

### Overhead

- Minimal performance overhead, only on errors
- No impact on happy path performance
- Async error handling to prevent blocking
- Error handlers should be efficient

### Best Practices

- Error handlers should be non-blocking
- Avoid recursive export attempts in error handler
- Use goroutines for expensive error handling operations
- Implement rate limiting for alert mechanisms
- Consider using buffered channels for high-volume error handling

## Alternative Approaches

Besides the error handler pattern, there are other approaches to improve robustness:

### 1. Channel-Based Error Handling

Channel-based error handling uses Go channels to communicate export errors. Instead of handling errors directly in the processor, errors are
sent through channels that can be consumed by multiple goroutines.

```go
// Channel-based approach
type ChannelErrorReporter struct {
 errorCh chan<- ExportError
}

type ExportError struct {
 Records []Record
 Error   error
 Context ExportErrorContext
}

// In processor
func (b *BatchProcessor) export(ctx context.Context, records []Record) {
 if err := b.exporter.Export(ctx, records); err != nil {
  select {
  case b.errorReporter.errorCh <- ExportError{
   Records: records,
   Error:   err,
   Context: ExportErrorContext{ProcessorType: "batch"},
  }:
  case <-ctx.Done():
   // Context cancelled, don't block
  }
 }
}

// Consumer goroutine
go func() {
 for exportError := range errorCh {
  handleExportError(exportError)
 }
}()
```

**Pros:**

- Decouples error detection from handling
- Multiple consumers can handle errors
- Non-blocking error reporting
- Natural Go pattern with channels

**Cons:**

- Requires goroutine management
- Channel buffer size considerations
- More complex setup

### 2. Middleware Pattern with Exporters

Implement a middleware pattern that wraps exporters:

```go
type ExporterMiddleware func(Exporter) Exporter

type ErrorHandlingExporter struct {
 next    Exporter
 handler ExportErrorHandler
}

func WithErrorHandling(handler ExportErrorHandler) ExporterMiddleware {
 return func(next Exporter) Exporter {
  return &ErrorHandlingExporter{
   next:    next,
   handler: handler,
  }
 }
}

func (e *ErrorHandlingExporter) Export(ctx context.Context, records []Record) error {
 if err := e.next.Export(ctx, records); err != nil {
  e.handler.HandleExportError(ctx, records, err)
  return err
 }
 return nil
}

// Usage
exporter = WithErrorHandling(myErrorHandler)(baseExporter)
processor := log.NewBatchProcessor(exporter)
```

**Pros:**

- Composable middleware pattern
- Reusable across different processors
- Clean separation of concerns

**Cons:**

- Extra layer of indirection
- More complex exporter chain
- Potential performance overhead

### 3. Circuit Breaker Pattern

Implement a circuit breaker for export operations:

```go
type CircuitBreakerExporter struct {
 next          Exporter
 breaker       *CircuitBreaker
 fallbackStore FallbackStorage
}

func (c *CircuitBreakerExporter) Export(ctx context.Context, records []Record) error {
 if c.breaker.State() == CircuitBreakerOpen {
  // Circuit is open - store in fallback
  return c.fallbackStore.Store(ctx, records)
 }

 err := c.next.Export(ctx, records)
 if err != nil {
  c.breaker.RecordFailure()
  // Fallback storage for failed records
  c.fallbackStore.Store(ctx, records)
 } else {
  c.breaker.RecordSuccess()
 }
 return err
}
```

**Pros:**

- Prevents cascade failures
- Automatic recovery mechanisms
- Protects downstream systems

**Cons:**

- Complex configuration and tuning
- Data loss during open circuit
- Additional infrastructure required

### 4. Retry Mechanism with Exponential Backoff

Built-in retry logic in exporters:

```go
type RetryableExporter struct {
 next       Exporter
 retryPolicy RetryPolicy
}

type RetryPolicy struct {
 MaxAttempts int
 BaseDelay   time.Duration
 MaxDelay    time.Duration
 Multiplier  float64
}

func (r *RetryableExporter) Export(ctx context.Context, records []Record) error {
 var lastErr error

 for attempt := 0; attempt < r.retryPolicy.MaxAttempts; attempt++ {
  if err := r.next.Export(ctx, records); err != nil {
   lastErr = err

   if attempt < r.retryPolicy.MaxAttempts-1 {
    delay := r.calculateBackoff(attempt)
    select {
    case <-time.After(delay):
     continue
    case <-ctx.Done():
     return ctx.Err()
    }
   }
  } else {
   return nil // Success
  }
 }

 return fmt.Errorf("export failed after %d attempts: %w",
  r.retryPolicy.MaxAttempts, lastErr)
}
```

**Pros:**

- Automatic retry for transient failures
- Configurable backoff strategies
- Reduces failure rates

**Cons:**

- Increased latency during failures
- Potential for blocking on persistent failures
- Complex retry policy configuration

### 5. Dead Letter Queue Pattern

Use a dead letter queue for failed exports:

```go
type DeadLetterQueueProcessor struct {
 next              Processor
 deadLetterQueue   chan []Record
 retryScheduler    *time.Ticker
 fallbackExporter  Exporter
}

func (d *DeadLetterQueueProcessor) OnEmit(ctx context.Context, record *Record) error {
 if err := d.next.OnEmit(ctx, record); err != nil {
  // Add to dead letter queue
  select {
  case d.deadLetterQueue <- []Record{*record}:
  default:
   // Queue full, handle overflow
   d.handleQueueOverflow(record)
  }
  return err
 }
 return nil
}

func (d *DeadLetterQueueProcessor) retryFromDeadLetter() {
 for {
  select {
  case <-d.retryScheduler.C:
   // Attempt to retry failed records
   d.processDeadLetterQueue()
  }
 }
}
```

**Pros:**

- No data loss
- Asynchronous retry processing
- Decoupled from main export flow

**Cons:**

- Memory overhead for queue
- Complex queue management
- Potential for unbounded queue growth

### 6. Metrics-Based Monitoring Approach

Focus on comprehensive metrics rather than direct error handling:

```go
type MetricsEnhancedProcessor struct {
 next         Processor
 metrics      ProcessorMetrics
}

type ProcessorMetrics struct {
 ExportAttempts    prometheus.Counter
 ExportSuccesses   prometheus.Counter
 ExportFailures    prometheus.CounterVec
 ExportDuration    prometheus.Histogram
 QueueSize         prometheus.Gauge
}

func (m *MetricsEnhancedProcessor) OnEmit(ctx context.Context, record *Record) error {
 start := time.Now()
 m.metrics.ExportAttempts.Inc()

 err := m.next.OnEmit(ctx, record)
 duration := time.Since(start)
 m.metrics.ExportDuration.Observe(duration.Seconds())

 if err != nil {
  m.metrics.ExportFailures.WithLabelValues(classifyError(err)).Inc()
  m.checkFailureThresholds()
 } else {
  m.metrics.ExportSuccesses.Inc()
 }

 return err
}
```

**Pros:**

- Comprehensive observability
- Integration with monitoring systems
- Proactive alerting based on metrics

**Cons:**

- No direct error handling
- Depends on external monitoring
- Logs can still be lost

## Hybrid Approaches

You can also combine patterns for comprehensive solutions:

### Recommended Combination for Enterprise Environments

```go
// Combine Error Handler + Circuit Breaker + Metrics + Retry
exporter := WithMetrics(
 WithCircuitBreaker(
  WithRetry(
   WithErrorHandling(auditLogErrorHandler)(baseExporter),
   retryPolicy,
  ),
  circuitBreakerConfig,
 ),
 metricsConfig,
)

processor := log.NewBatchProcessor(exporter,
 log.WithExportErrorHandler(globalErrorHandler),
)
```

This combination offers:

- Error handler for application-specific logic
- Circuit breaker for system protection
- Retry mechanism for transient failures
- Comprehensive metrics for monitoring

## Conclusion

The error handler pattern implementation makes the OpenTelemetry Go Client significantly more robust and observable. Developers can react to
export problems and protect critical log data while maintaining Go's idiomatic patterns and excellent performance characteristics.

The implementation follows established Go patterns and integrates smoothly into OpenTelemetry, while opening new possibilities for
enterprise-grade audit logging and observability requirements.

The flexible design allows for incremental adoption and can be combined with other robustness patterns as needed for specific use cases.
