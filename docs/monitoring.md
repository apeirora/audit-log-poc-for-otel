# Monitoring Stack Setup Guide

This guide explains how to install and manage Prometheus and Grafana monitoring stack with authentication for the OpenTelemetry Audit Log PoC project.

## Overview

The monitoring stack includes:
- **Prometheus**: Metrics collection and storage with AlertManager
- **Grafana**: Visualization and dashboarding with authentication
- **Node Exporter**: System metrics collection
- **Kube State Metrics**: Kubernetes cluster metrics
- **Prometheus Operator**: Kubernetes-native Prometheus management

## Prerequisites

- A running Kubernetes cluster (k3d or kind)
- Helm 3.x installed
- kubectl configured to access the cluster

## Quick Start

### 1. Install the Complete Monitoring Stack

```bash
# Install both Prometheus and Grafana
task monitoring:install
```

This will:
- Create a `monitoring` namespace
- Install Prometheus with persistent storage
- Install Grafana with authentication enabled
- Configure Grafana to use Prometheus as a data source
- Set up default dashboards

### 2. Access the Services

#### Grafana (Port 3000)
```bash
# Port-forward Grafana to localhost:3000
task monitoring:port-forward-grafana
```

Default credentials:
- **Username**: `admin`
- **Password**: Check `.tools/grafana-password` or run `task monitoring:get-grafana-password`

#### Prometheus (Port 9090)
```bash
# Port-forward Prometheus to localhost:9090
task monitoring:port-forward-prometheus
```

#### AlertManager (Port 9093)
```bash
# Port-forward AlertManager to localhost:9093
task monitoring:port-forward-alertmanager
```

## Available Tasks

### Installation and Management

```bash
# Install complete monitoring stack
task monitoring:install

# Install only Prometheus
task monitoring:install-prometheus

# Install only Grafana
task monitoring:install-grafana

# Check status of monitoring components
task monitoring:status

# Upgrade the monitoring stack
task monitoring:upgrade

# Uninstall the monitoring stack
task monitoring:uninstall
```

### Access and Monitoring

```bash
# Port-forward services
task monitoring:port-forward-grafana      # Access at http://localhost:3000
task monitoring:port-forward-prometheus   # Access at http://localhost:9090
task monitoring:port-forward-alertmanager # Access at http://localhost:9093

# Get Grafana admin password
task monitoring:get-grafana-password

# View logs
task monitoring:logs-grafana
task monitoring:logs-prometheus
```

### Maintenance

```bash
# Clean persistent volumes (WARNING: Deletes all data!)
task monitoring:clean-volumes
```

## Configuration

### Helm Values Override Files

The monitoring stack uses Helm values override files for configuration:

- **`helm/prometheus-stack-overrides.yaml`**: Prometheus stack configuration
- **`helm/grafana-overrides.yaml`**: Grafana configuration with authentication

### Key Configuration Features

#### Prometheus Configuration
- 30-day data retention
- 10Gi persistent storage
- Resource limits configured
- Service discovery for all namespaces
- AlertManager with 2Gi storage

#### Grafana Configuration
- Admin authentication enabled
- 5Gi persistent storage
- Pre-configured Prometheus datasource
- Default dashboards for Kubernetes monitoring
- Plugin support for enhanced visualizations

### Authentication

Grafana is configured with basic authentication:
- Admin user is enabled by default
- Password is randomly generated and stored in `.tools/grafana-password`
- Anonymous access is disabled
- User sign-up is disabled

## Default Dashboards

Grafana comes pre-configured with several dashboards:

### Kubernetes Dashboards
- **Node Exporter Dashboard** (ID: 1860): System metrics
- **Kubernetes Cluster Monitoring** (ID: 7249): Cluster overview
- **Kubernetes Pod Monitoring** (ID: 6417): Pod-level metrics
- **Kubernetes Deployment** (ID: 8588): Deployment metrics

### OpenTelemetry Dashboards
- **OpenTelemetry Collector** (ID: 15983): OTel Collector metrics
- **Jaeger Dashboard** (ID: 10001): Tracing metrics

## Storage

The monitoring stack uses persistent storage:

- **Prometheus**: 10Gi for metrics storage
- **AlertManager**: 2Gi for alert storage
- **Grafana**: 5Gi for dashboard and configuration storage

Storage class is configured as `local-path` (suitable for local development clusters).

## Security

### Grafana Security Features
- Admin user authentication
- Secure session management
- CSRF protection enabled
- Content security headers configured
- Login brute force protection

### Prometheus Security
- RBAC enabled
- Security contexts configured
- Non-root user execution
- Network policies can be added

## Troubleshooting

### Common Issues

1. **Pods not starting**
   ```bash
   # Check pod status
   kubectl get pods -n monitoring
   
   # Check events
   kubectl get events -n monitoring --sort-by='.lastTimestamp'
   ```

2. **Storage issues**
   ```bash
   # Check persistent volume claims
   kubectl get pvc -n monitoring
   
   # Check storage class
   kubectl get storageclass
   ```

3. **Service connectivity issues**
   ```bash
   # Check services
   kubectl get svc -n monitoring
   
   # Test service connectivity
   kubectl run test-pod --image=busybox -it --rm --restart=Never -- wget -qO- http://prometheus-kube-prometheus-prometheus.monitoring.svc.cluster.local:9090
   ```

### Logs and Debugging

```bash
# View Grafana logs
task monitoring:logs-grafana

# View Prometheus logs
task monitoring:logs-prometheus

# Get detailed status
task monitoring:status
```

### Reset and Clean Installation

If you need to start fresh:

```bash
# Uninstall monitoring stack
task monitoring:uninstall

# Clean persistent volumes (WARNING: This deletes all data!)
task monitoring:clean-volumes

# Reinstall
task monitoring:install
```

## Integration with OTEL Demo

The monitoring stack integrates seamlessly with the existing OTEL demo:

1. **Service Discovery**: Prometheus automatically discovers services with proper annotations
2. **Metrics Collection**: Collects metrics from all OTEL components
3. **Alerting**: Pre-configured alerts for common issues
4. **Dashboards**: Specialized dashboards for OpenTelemetry components

## Customization

### Adding Custom Dashboards

1. Export dashboard JSON from Grafana UI
2. Place in `helm/dashboards/` directory
3. Update `helm/grafana-overrides.yaml` to include the dashboard

### Adding Custom Alerts

1. Create PrometheusRule custom resources
2. Apply to the monitoring namespace
3. Alerts will be automatically picked up by AlertManager

### Adding Custom Data Sources

Update the `datasources` section in `helm/grafana-overrides.yaml`:

```yaml
datasources:
  datasources.yaml:
    apiVersion: 1
    datasources:
    - name: MyCustomSource
      type: prometheus
      url: http://my-prometheus:9090
      access: proxy
```

## Production Considerations

For production deployments, consider:

1. **Security**: Enable TLS, configure proper RBAC, use secrets for passwords
2. **Storage**: Use production-grade storage classes with backup
3. **High Availability**: Enable multiple replicas for critical components
4. **Resource Limits**: Adjust resource limits based on your workload
5. **Networking**: Configure ingress controllers and network policies
6. **Monitoring**: Monitor the monitoring stack itself

## Support

For issues and questions:
1. Check the troubleshooting section above
2. Review Helm chart documentation:
   - [Prometheus Stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
   - [Grafana](https://github.com/grafana/helm-charts/tree/main/charts/grafana)
3. Check project issues and documentation