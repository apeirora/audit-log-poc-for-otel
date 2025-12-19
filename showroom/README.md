# End-to-End OpenTelemetry Logs

This folder contains a Proof of Concept (PoC) for sending OpenTelemetry logs.

## Prerequisites

- [`task`](https://taskfile.dev/) installed on your local machine.
- A Garden cluster.

## Accounts

- [Garden shoot: otel-audit-log](https://dashboard.ingress.garden.gardener.cc-one.showroom.apeirora.eu/namespace/garden-msp06/shoots/otel-audit-log)

## Deployment

1. Clone the repository:

  ```bash
  git clone https://github.com/apeirora/audit-log-poc-for-otel.git
  cd audit-log-poc-for-otel
  git checkout showroom
  cd showroom
  ```

1. Check your Garden cluster access:

  ```bash
  task
  ```

1. Deploy the whole stack:

  ```bash
  task deploy
  ```
