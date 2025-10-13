package com.example.otlpreceiver;

import java.util.ArrayList;
import java.util.Collection;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.atomic.AtomicLong;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;

import com.google.protobuf.GeneratedMessage;
import com.google.protobuf.UnknownFieldSet;

import io.grpc.stub.StreamObserver;
import io.opentelemetry.proto.collector.logs.v1.ExportLogsServiceRequest;
import io.opentelemetry.proto.collector.logs.v1.ExportLogsServiceResponse;
import io.opentelemetry.proto.collector.logs.v1.LogsServiceGrpc;
import io.opentelemetry.proto.common.v1.KeyValue;
import io.opentelemetry.proto.logs.v1.LogRecord;
import io.opentelemetry.proto.logs.v1.ResourceLogs;
import io.opentelemetry.proto.logs.v1.ScopeLogs;
import io.opentelemetry.proto.resource.v1.Resource;

@Service
public class LogsServiceImpl extends LogsServiceGrpc.LogsServiceImplBase {

  private final Logger log = LoggerFactory.getLogger(LogsServiceImpl.class);
  private final Collection<LogRecord> receivedLogs = new LinkedBlockingQueue<>();
  private final AtomicLong logCount = new AtomicLong(0);
  private final AtomicLong largestLogNo = new AtomicLong(0);

  public long getTotalLogCount() {
    return logCount.get();
  }

  @Override
  public void export(ExportLogsServiceRequest request, StreamObserver<ExportLogsServiceResponse> responseObserver) {
    logUnknownFields(request);
    log.debug("Received {} resource logs", request.getResourceLogsCount());

    List<ResourceLogs> resourceLogs = request.getResourceLogsList();
    log.info("Resource Logs count: {}", resourceLogs.size());

    resourceLogs.forEach(rLog -> {
      logUnknownFields(rLog);
      List<ScopeLogs> scopeLogs = rLog.getScopeLogsList();
      log.info("Instrumentation Library Logs count: {}", scopeLogs.size());
      Resource resource = rLog.getResource();
      logUnknownFields(resource);
      log.debug("Resource Attributes: {}", resource.getAttributesList());
      scopeLogs.forEach(sLog -> {
        logUnknownFields(sLog);
        log.debug("Scope: {}", sLog.getScope());
        List<LogRecord> logs = sLog.getLogRecordsList();
        log.info("Log Records count: {}", logs.size());
        logCount.addAndGet(logs.size());
        receivedLogs.addAll(logs);
        logs.forEach(logRecord -> {
          logUnknownFields(logRecord);
          long logNo = getLogNo(logRecord);
          largestLogNo.updateAndGet(x -> Math.max(x, logNo));
          log.info("total: {}, current {}, max seen {}", getTotalLogCount(), logNo, largestLogNo.get());
        });
      });
    });

    ExportLogsServiceResponse response = ExportLogsServiceResponse.newBuilder().build();
    responseObserver.onNext(response);
    responseObserver.onCompleted();

    log.info("Received in total {} logs", getTotalLogCount());
    log.info("Received {} new logs, total is now {}", request.getResourceLogsCount(), getTotalLogCount());
    // log.info("Received {} logs:\n{}", receivedLogs.size(), receivedLogs);

    // Optionally, print each log in a more readable format
    // receivedLogs.forEach(logRecord -> log.info(toString(logRecord)));

  }

  public List<LogRecord> getReceivedLogs() {
    return new ArrayList<>(receivedLogs);
  }

  public void clear() {
    receivedLogs.clear();
  }

  public String toString(LogRecord logRecord) {
    StringBuilder sb = new StringBuilder();
    sb.append("LogRecord{");
    sb.append("timeUnixNano=").append(logRecord.getTimeUnixNano());
    sb.append(", severityText=").append(logRecord.getSeverityText());
    sb.append(", severityNumber=").append(logRecord.getSeverityNumber().getNumber());
    sb.append(", body=").append(logRecord.getBody().getStringValue());
    sb.append(", attributes=").append(logRecord.getAttributesList());
    sb.append('}');
    return sb.toString();
  }

  public long getLogNo(LogRecord logRecord) {
    List<KeyValue> attributes = logRecord.getAttributesList();
    if (attributes == null) {
      return 0;
    }
    Optional<KeyValue> logno = attributes.stream().filter(attr -> attr.getKey().equals("logNo#")).findFirst();
    if (logno.isPresent()) {
      return logno.get().getValue().getIntValue();
    } else {
      return 0;
    }
  }
  
  /**
   * Log any unknown fields present in the given GeneratedMessage.
   * 
   * @param message
   */
  private void logUnknownFields(GeneratedMessage message) {
    try {
      Map<Integer, UnknownFieldSet.Field> unknownFields = message.getUnknownFields().asMap();
      unknownFields.forEach((k, v) -> {
        log.warn("Unknown field in {}: key={}, value={}", message.getClass().getSimpleName(), k, v);
      });
    } catch (Exception e) {
      log.warn("Failed to get unknown fields from: " + message.getClass().getSimpleName(), e);
    }
  }
}