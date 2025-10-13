package com.sap.otel.demo;

import java.io.File;
import java.time.Instant;
import java.util.Random;
import java.util.concurrent.atomic.AtomicInteger;

import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RestController;

import io.opentelemetry.api.common.AttributeKey;
import io.opentelemetry.api.logs.Logger;
import io.opentelemetry.api.logs.LoggerProvider;
import io.opentelemetry.api.logs.Severity;
import io.opentelemetry.contrib.disk.buffering.internal.storage.FileSpanStorage;
import io.opentelemetry.contrib.disk.buffering.storage.SignalStorage;

/**
 * REST controller providing a /rolldice endpoint similar to the Go example. Uses OpenTelemetry for
 * audit logging.
 */
@RestController
public class DiceController {

  private final Random random = new Random();
  private final Logger otelLogger;
  private final org.slf4j.Logger log = LoggerFactory.getLogger(getClass());
  private final AtomicInteger logCount = new AtomicInteger(0);
  // Number of log messages per request, configurable via environment variable:
  // LOG_MESSAGES_PER_REQUEST
  private final int logMessagesPerRequest;

  public int getTotalLogCount() {
    return logCount.get();
  }

  public DiceController(
      @Value("${LOG_MESSAGES_PER_REQUEST:10}") int logMessagesPerRequest,
      @Qualifier("auditLoggerProvider") @Autowired LoggerProvider loggerProvider) {
    log.debug("DiceController initialized with logMessagesPerRequest={}", logMessagesPerRequest);
    log.info("Using LoggerProvider implementation: {}", loggerProvider.getClass().getName());
    // LoggerProvider loggerProvider = GlobalOpenTelemetry.get().getLogsBridge(); // actually
    // returns LoggerProvider.noop()
    this.otelLogger = loggerProvider.loggerBuilder("AUDIT_JAVA_SERVICE").build();
    this.logMessagesPerRequest = logMessagesPerRequest;
  }

  /**
   * Endpoint: /rolldice/{player} Rolls a dice and logs the action using OpenTelemetry.
   *
   * @param player the player name (optional)
   * @return the dice roll result
   */
  @GetMapping({"/rolldice", "/rolldice/{player}"})
  public String rollDice(@PathVariable(value = "player", required = false) String player) {
    log.debug("rollDice called with player={}", player);
    int roll = 1 + random.nextInt(100);
    String user = player != null ? player : "Anonymous";
    String msg = user + " is rolling the dice";

    // Audit log with OpenTelemetry
    otelLogger
        .logRecordBuilder()
        .setSeverity(Severity.INFO)
        .setBody(msg)
        .setAttribute(AttributeKey.stringKey("result"), String.valueOf(roll))
        .setAttribute(AttributeKey.stringKey("AUDIT-USER"), user)
        .setAttribute(AttributeKey.longKey("logNo#"), (long)logCount.incrementAndGet())
        .setTimestamp(Instant.now())
        .emit();
    

    // Emit additional log messages if configured
    for (int i = 0; i < logMessagesPerRequest; i++) {
      otelLogger
          .logRecordBuilder()
          .setSeverity(Severity.INFO)
          .setBody(String.format("dice: %d, user: %s - bulk log message #%d", roll, user, i + 1))
          .setAttribute(AttributeKey.longKey("logNo#"), (long)logCount.incrementAndGet())
          .setTimestamp(Instant.now())
          .emit();
    }

    return roll + "\n";
  }
}
