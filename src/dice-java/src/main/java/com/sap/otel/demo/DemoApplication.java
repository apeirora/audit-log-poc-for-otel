package com.sap.otel.demo;

import org.slf4j.LoggerFactory;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class DemoApplication {

  public static void main(String[] args) {
    LoggerFactory.getLogger(DemoApplication.class)
        .info("Calling SpringApplication.run() to start the application");
    SpringApplication.run(DemoApplication.class, args);
  }
}
