package com.sap.otel.demo;

import java.io.IOException;
import java.nio.file.Path;
import java.time.Duration;
import java.util.concurrent.TimeUnit;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import io.opentelemetry.api.common.AttributeKey;
import io.opentelemetry.api.logs.LoggerProvider;
import io.opentelemetry.exporter.internal.otlp.logs.AuditLogFileStore;
import io.opentelemetry.exporter.logging.SystemOutLogRecordExporter;
import io.opentelemetry.exporter.otlp.http.logs.OtlpHttpLogRecordExporter;
import io.opentelemetry.exporter.otlp.logs.OtlpGrpcLogRecordExporter;
import io.opentelemetry.sdk.OpenTelemetrySdk;
import io.opentelemetry.sdk.common.export.RetryPolicy;
import io.opentelemetry.sdk.logs.LogRecordProcessor;
import io.opentelemetry.sdk.logs.SdkLoggerProvider;
import io.opentelemetry.sdk.logs.export.AuditException;
import io.opentelemetry.sdk.logs.export.AuditExceptionHandler;
import io.opentelemetry.sdk.logs.export.AuditLogRecordProcessor;
import io.opentelemetry.sdk.logs.export.AuditLogStore;
import io.opentelemetry.sdk.logs.export.BatchLogRecordProcessor;
import io.opentelemetry.sdk.logs.export.LogRecordExporter;
import io.opentelemetry.sdk.logs.export.SimpleLogRecordProcessor;
import io.opentelemetry.sdk.resources.Resource;

@Configuration
public class OpenTelemetryConfig {

  /**
   * Configures OpenTelemetry logging with OTLP exporters (gRPC and HTTP) and a stdout exporter. The service name is set to
   * "audit-java-service".
   *
   * @see org.springframework.boot.actuate.autoconfigure.logging.OpenTelemetryLoggingAutoConfiguration
   * @return LoggerProvider configured with OTLP exporters and stdout exporter
   */
  public LoggerProvider otelLoggerProvider() {
    // OTLP gRPC exporter
    String grpcEndpoint = System.getenv().getOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT_GRPC", "http://localhost:4317");
    System.out.println("Using OTLP gRPC endpoint: " + grpcEndpoint);
    LogRecordExporter grpcExporter = OtlpGrpcLogRecordExporter.builder().setEndpoint(grpcEndpoint).build();

    // OTLP HTTP exporter
    String httpEndpoint = System.getenv().getOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT_HTTP", "http://localhost:4318");
    System.out.println("Using OTLP HTTP endpoint: " + httpEndpoint);

    RetryPolicy retryPolicy = RetryPolicy.builder().setMaxAttempts(5).setInitialBackoff(Duration.ofSeconds(30))
        .setMaxBackoff(Duration.ofSeconds(30)).setBackoffMultiplier(3).build();

    LogRecordExporter httpExporter = OtlpHttpLogRecordExporter.builder().setEndpoint(httpEndpoint).setRetryPolicy(retryPolicy)
        .setConnectTimeout(Duration.ofSeconds(1)).build();

    // Stdout exporter
    LogRecordExporter stdoutExporter = SystemOutLogRecordExporter.create();

    Resource resource = Resource.getDefault().toBuilder().put(AttributeKey.stringKey("service.name"), "audit-java-service").build();

    SdkLoggerProvider loggerProvider = SdkLoggerProvider.builder()
        .addLogRecordProcessor(BatchLogRecordProcessor.builder(stdoutExporter).build())
        .addLogRecordProcessor(BatchLogRecordProcessor.builder(grpcExporter).build())
        .addLogRecordProcessor(BatchLogRecordProcessor.builder(httpExporter).build()).setResource(resource).build();

    // Optionally set as global
    OpenTelemetrySdk.builder().setLoggerProvider(loggerProvider).buildAndRegisterGlobal();
    return loggerProvider;
  }

  @Bean
  public LoggerProvider auditLoggerProvider() {
    // OTLP gRPC exporter
    RetryPolicy retryPolicy = RetryPolicy.builder().setMaxAttempts(5).setInitialBackoff(Duration.ofSeconds(30))
        .setMaxBackoff(Duration.ofSeconds(30)).setBackoffMultiplier(3).build();

    String grpcEndpoint = System.getenv().getOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT_GRPC", "http://localhost:4317");
    System.out.println("Using OTLP gRPC endpoint: " + grpcEndpoint);

    LogRecordExporter grpcExporter = OtlpGrpcLogRecordExporter.builder().setEndpoint(grpcEndpoint).setRetryPolicy(retryPolicy)
        .setConnectTimeout(Duration.ofSeconds(1)).build();

    Resource resource = Resource.getDefault().toBuilder().put(AttributeKey.stringKey("service.name"), "audit-java-service").build();

    // Audit log store using a temporary directory for local temporary persistence
    Path tmp = Path.of(System.getProperty("java.io.tmpdir"));
    AuditLogStore auditLogStore;
    try {
      auditLogStore = new AuditLogFileStore(tmp);
    } catch (IOException e) {
      // TODO Auto-generated catch block
      e.printStackTrace();
      throw new RuntimeException("Failed to create AuditLogFileStore", e);
    }
    AuditExceptionHandler auditExceptionHandler = new AuditExceptionHandler() {
      @Override
      public void handle(AuditException auditEx) {
        System.err.println("AuditException: " + auditEx.getMessage());
        auditEx.logRecords.forEach(lr -> System.err.println("  " + lr));
      }
    };
    LogRecordProcessor auditLogProcessor = AuditLogRecordProcessor.builder(grpcExporter, auditLogStore)
        .setExceptionHandler(auditExceptionHandler) // use default handler which logs to stderr
        .setExporterTimeout(10, TimeUnit.SECONDS) // use default timeout of 30s
        .setRetryPolicy(retryPolicy) // use default retry policy
        .setMaxExportBatchSize(20) // increase batch size for audit logs
        .setScheduleDelay(2, TimeUnit.SECONDS) // use default delay of 5s
        // .setWaitOnExport(true) // wait for export to complete
        .setWaitOnExport(false) // stay async
        .build();

    SdkLoggerProvider loggerProvider = SdkLoggerProvider.builder()
        .addLogRecordProcessor(SimpleLogRecordProcessor.create(SystemOutLogRecordExporter.create())) // also log to stdout
        .addLogRecordProcessor(auditLogProcessor) // add audit log processor for guaranteed delivery
        .setResource(resource).build();

    // Optionally set as global
    OpenTelemetrySdk.builder().setLoggerProvider(loggerProvider).buildAndRegisterGlobal();
    return loggerProvider;
  }
}
