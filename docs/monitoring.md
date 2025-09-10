# Monitoring Stack Setup Guide

This guide explains how to install and manage Prometheus and Grafana monitoring stack with authentication for the OpenTelemetry Audit Log
PoC project.

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
- Install Prometheus
- Install Grafana with anonymous access
- Configure Grafana to use Prometheus as a data source

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
