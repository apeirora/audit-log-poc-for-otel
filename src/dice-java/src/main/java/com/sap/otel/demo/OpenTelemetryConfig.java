package com.sap.otel.demo;

import io.opentelemetry.api.common.AttributeKey;
import io.opentelemetry.api.logs.LoggerProvider;
import io.opentelemetry.exporter.logging.SystemOutLogRecordExporter;
import io.opentelemetry.exporter.otlp.http.logs.OtlpHttpLogRecordExporter;
import io.opentelemetry.exporter.otlp.logs.OtlpGrpcLogRecordExporter;
import io.opentelemetry.sdk.OpenTelemetrySdk;
import io.opentelemetry.sdk.common.export.RetryPolicy;
import io.opentelemetry.sdk.logs.SdkLoggerProvider;
import io.opentelemetry.sdk.logs.export.BatchLogRecordProcessor;
import io.opentelemetry.sdk.logs.export.LogRecordExporter;
import io.opentelemetry.sdk.resources.Resource;
import java.time.Duration;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class OpenTelemetryConfig {

  /**
   * Configures OpenTelemetry logging with OTLP exporters (gRPC and HTTP) and a stdout exporter. The
   * service name is set to "audit-java-service".
   *
   * @see
   *     org.springframework.boot.actuate.autoconfigure.logging.OpenTelemetryLoggingAutoConfiguration
   * @return LoggerProvider configured with OTLP exporters and stdout exporter
   */
  @Bean
  public LoggerProvider otelLoggerProvider() {
    // OTLP gRPC exporter
    String grpcEndpoint =
        System.getenv().getOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT_GRPC", "http://localhost:4317");
    System.out.println("Using OTLP gRPC endpoint: " + grpcEndpoint);
    LogRecordExporter grpcExporter =
        OtlpGrpcLogRecordExporter.builder().setEndpoint(grpcEndpoint).build();

    // OTLP HTTP exporter
    String httpEndpoint =
        System.getenv().getOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT_HTTP", "http://localhost:4318");
    System.out.println("Using OTLP HTTP endpoint: " + httpEndpoint);

    RetryPolicy retryPolicy =
        RetryPolicy.builder()
            .setMaxAttempts(5)
            .setInitialBackoff(Duration.ofSeconds(30))
            .setMaxBackoff(Duration.ofSeconds(30))
            .setBackoffMultiplier(3)
            .build();

    LogRecordExporter httpExporter =
        OtlpHttpLogRecordExporter.builder()
            .setEndpoint(httpEndpoint)
            .setRetryPolicy(retryPolicy)
            .setConnectTimeout(Duration.ofSeconds(1))
            .build();

    // Stdout exporter
    LogRecordExporter stdoutExporter = SystemOutLogRecordExporter.create();

    Resource resource =
        Resource.getDefault().toBuilder()
            .put(AttributeKey.stringKey("service.name"), "audit-java-service")
            .build();

    SdkLoggerProvider loggerProvider =
        SdkLoggerProvider.builder()
            .addLogRecordProcessor(BatchLogRecordProcessor.builder(stdoutExporter).build())
            .addLogRecordProcessor(BatchLogRecordProcessor.builder(grpcExporter).build())
            .addLogRecordProcessor(BatchLogRecordProcessor.builder(httpExporter).build())
            .setResource(resource)
            .build();

    // Optionally set as global
    OpenTelemetrySdk.builder().setLoggerProvider(loggerProvider).buildAndRegisterGlobal();
    return loggerProvider;
  }
}
