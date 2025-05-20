[![REUSE status](https://api.reuse.software/badge/github.com/apeirora/audit-log-poc-for-otel)](https://api.reuse.software/info/github.com/apeirora/audit-log-poc-for-otel)

# Audit Log Proof of Concept for OpenTelemetry

Audit-logging with OTel - how could this work?

## About This Project

This PoC scenario provides an easy setup of an [OpenTelemetry Demo](https://opentelemetry.io/docs/demo/) environment to test how log messages are delivered through multiple different systems.

## Requirements and Setup

* [git](https://git-scm.com/)
* [docker](https://www.docker.com/) or [podman](https://podman.io/)
* [task](https://taskfile.dev/)
* optional: [k9s](https://k9scli.io/)

### Quick Start Linux

```bash
git clone https://github.com/apeirora/otel-audit-log-poc.git
cd otel-audit-log-poc
sudo snap install task --classic
task otel:demo
# wait a couple of minutes until all images are downloaded and pods are running - feel free to check with k9s
task otel:port-forward
```

### Demo Endpoints

With the frontend-proxy port-forward set up, you can access:

Web store: http://localhost:8080/
Grafana: http://localhost:8080/grafana/
Load Generator UI: http://localhost:8080/loadgen/
Jaeger UI: http://localhost:8080/jaeger/ui/
Flagd configurator UI: http://localhost:8080/feature

## Support, Feedback, Contributing

This project is open to feature requests/suggestions, bug reports etc. via [GitHub issues](https://github.com/apeirora/audit-log-poc-for-otel/issues). Contribution and feedback are encouraged and always welcome. For more information about how to contribute, the project structure, as well as additional contribution information, see our [Contribution Guidelines](CONTRIBUTING.md).

## Security / Disclosure

If you find any bug that may be a security problem, please follow our instructions at [in our security policy](https://github.com/apeirora/audit-log-poc-for-otel/security/policy) on how to report it. Please do not create GitHub issues for security-related doubts or problems.

## Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/apeirora/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## Licensing

Copyright 2025 SAP SE or an SAP affiliate company and ApeiroRA contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/apeirora/audit-log-poc-for-otel).
