package com.example.otlpreceiver;

import io.grpc.Server;
import io.grpc.ServerBuilder;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import javax.annotation.PostConstruct;
import javax.annotation.PreDestroy;
import java.io.IOException;

@Component
public class GrpcServer {

  private final Server server;
  
  private final Logger log = LoggerFactory.getLogger(GrpcServer.class);

  public GrpcServer(@Value("${grpc.port:4317}") int port) {
    this.server = ServerBuilder.forPort(port)
        .addService(new LogsServiceImpl())
        .build();
    
    log.info("gRPC server initialized on port {}", port);
  }

  @PostConstruct
  public void start() throws IOException {
    server.start();
    log.info("gRPC server started, listening on {}", server.getPort());
  }

  @PreDestroy
  public void stop() {
    if (server != null) {
      server.shutdown();
    }
    log.info("gRPC server stopped");
  }

  public void blockUntilShutdown() throws InterruptedException {
    if (server != null) {
      server.awaitTermination();
    }
  }
}