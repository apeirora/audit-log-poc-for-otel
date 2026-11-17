#!/bin/sh
set -eu
if [ "${ROLE}" = "sidecar" ]; then
  /usr/local/bin/edge -mode sign &
  exec /usr/local/bin/otelcol-contrib --config /etc/otel/sidecar.yaml
fi
/usr/local/bin/edge -mode verify &
exec /usr/local/bin/otelcol-contrib --config /etc/otel/collector.yaml
