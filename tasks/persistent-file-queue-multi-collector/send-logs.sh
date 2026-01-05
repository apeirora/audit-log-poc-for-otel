#!/usr/bin/env bash
set -euo pipefail

# Send 10 OTLP/HTTP logs to each collector service on the compose network.
# Requires docker and docker compose; run from this directory after `docker compose up -d`.

NET=$(docker compose ps -q otelcol1 | xargs -r docker inspect -f '{{range $k,$v := .NetworkSettings.Networks}}{{ $k }}{{end}}' || true)
if [ -z "${NET}" ]; then
  NET=$(docker compose ps -q | head -n1 | xargs -r docker inspect -f '{{range $k,$v := .NetworkSettings.Networks}}{{ $k }}{{end}}' || true)
fi
if [ -z "${NET}" ]; then
  echo "Could not determine compose network; is docker compose up -d running?" >&2
  exit 1
fi

echo "Using docker network: ${NET}"

for collector in otelcol1 otelcol2 otelcol3; do
  for i in $(seq 1 10); do
    ts=$(date +%s%N)
    payload=$(cat <<EOF
{
  "resourceLogs": [{
    "resource": {"attributes":[{"key":"service.name","value":{"stringValue":"loadgen"}}]},
    "scopeLogs": [{
      "scope": {},
      "logRecords": [{
        "timeUnixNano": "${ts}",
        "severityNumber": 9,
        "severityText": "INFO",
        "body": {"stringValue": "test log ${i} for ${collector}"}
      }]
    }]
  }]
}
EOF
)
    status=$(docker run --rm --network "$NET" curlimages/curl:8.8.0 \
      -s -S -o /dev/null -w "%{http_code}" \
      --retry 5 --retry-delay 1 --retry-all-errors --max-time 5 \
      -X POST "http://${collector}:4318/v1/logs" \
      -H "Content-Type: application/json" \
      -d "$payload" 2>&1) || true
    if [ "${status}" != "200" ]; then
      echo "collector=${collector} log=${i} status=${status}"
    fi
  done
done

