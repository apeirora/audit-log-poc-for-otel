package com.example.otlpreceiver;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

@SpringBootApplication
public class OtlpReceiverApplication {

  public static void main(String[] args) {
    SpringApplication.run(OtlpReceiverApplication.class, args);
  }

  @Bean
  public CommandLineRunner commandLineRunner(GrpcServer grpcServer) {
    return args -> {
      grpcServer.blockUntilShutdown();
    };
  }
}