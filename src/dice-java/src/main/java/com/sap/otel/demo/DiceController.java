package com.sap.otel.demo;

import io.opentelemetry.api.common.AttributeKey;
import io.opentelemetry.api.logs.Logger;
import io.opentelemetry.api.logs.LoggerProvider;
import io.opentelemetry.api.logs.Severity;
import java.util.Random;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RestController;

/**
 * REST controller providing a /rolldice endpoint similar to the Go example. Uses OpenTelemetry for
 * audit logging.
 */
@RestController
public class DiceController {

  private final Random random = new Random();
  private final Logger otelLogger;
  private final org.slf4j.Logger log = LoggerFactory.getLogger(getClass());

  // Number of log messages per request, configurable via environment variable:
  // LOG_MESSAGES_PER_REQUEST
  private final int logMessagesPerRequest;

  public DiceController(
      @Value("${LOG_MESSAGES_PER_REQUEST:1}") int logMessagesPerRequest,
      @Qualifier("otelLoggerProvider") @Autowired LoggerProvider loggerProvider) {
    log.debug("DiceController initialized with logMessagesPerRequest={}", logMessagesPerRequest);
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
        .emit();

    // Emit additional log messages if configured
    for (int i = 0; i < logMessagesPerRequest; i++) {
      otelLogger
          .logRecordBuilder()
          .setSeverity(Severity.INFO)
          .setBody(String.format("dice: %d, user: %s - bulk log message #%d", roll, user, i + 1))
          .emit();
    }

    return roll + "\n";
  }
}
