#!/usr/bin/env bash
set -euo pipefail

NS=${NS:-otel-demo}
KCTX=${KCTX:-kind-otel-demo}
LOG_COUNT=${LOG_COUNT:-10}
PORT_BASE=${PORT_BASE:-14318}
collectors=(otelcol1 otelcol2 otelcol3)

if ! kubectl --context "${KCTX}" get ns "${NS}" >/dev/null 2>&1; then
  echo "kubectl context ${KCTX} not found or namespace ${NS} unreachable."
  echo "Set KUBECONFIG to your kubeconfig (e.g., /mnt/c/Users/<you>/.kube/config) and rerun."
  exit 1
fi

for idx in "${!collectors[@]}"; do
  collector=${collectors[$idx]}
  port=$((PORT_BASE + idx * 1000))
  echo "Port-forwarding ${collector} to localhost:${port}"
  kubectl --context "${KCTX}" port-forward -n "${NS}" deploy/"${collector}" "${port}:4318" >/tmp/port-forward-"${collector}".log 2>&1 &
  pf_pid=$!
  ready=false
  for _ in $(seq 1 20); do
    if ! ps -p "${pf_pid}" >/dev/null 2>&1; then
      break
    fi
    code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 1 "http://127.0.0.1:${port}/v1/logs" || true)
    if [ "${code}" != "000" ]; then
      ready=true
      break
    fi
    sleep 0.5
  done
  if [ "${ready}" != true ]; then
    echo "port-forward for ${collector} did not become ready; log:"
    cat /tmp/port-forward-"${collector}".log || true
    kill "${pf_pid}" 2>/dev/null || true
    wait "${pf_pid}" 2>/dev/null || true
    continue
  fi

  echo "Sending ${LOG_COUNT} logs to ${collector}"
  for i in $(seq 1 "${LOG_COUNT}"); do
    ts=$(date +%s%N)
    payload=$(cat <<EOF
{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"loadgen"}}]},"scopeLogs":[{"scope":{},"logRecords":[{"timeUnixNano":"${ts}","severityNumber":9,"severityText":"INFO","body":{"stringValue":"test log ${i} for ${collector}"}}]}]}]}
EOF
)
    status=$(curl -s -S -o /dev/null -w "%{http_code}" \
      --retry 5 --retry-delay 1 --retry-all-errors --max-time 5 \
      -X POST "http://127.0.0.1:${port}/v1/logs" \
      -H "Content-Type: application/json" \
      -d "${payload}" 2>&1) || true
    if [ "${status}" != "200" ]; then
      echo "collector=${collector} log=${i} status=${status}"
    fi
  done

  kill "${pf_pid}" 2>/dev/null || true
  wait "${pf_pid}" 2>/dev/null || true

  echo "Storage usage for ${collector} (via ephemeral debug container)"
  kubectl --context "${KCTX}" debug -n "${NS}" deploy/"${collector}" --image=alpine:3.19 --target=otelcol --quiet -- sh -c "ls -ld /data/queue-* && du -sh /data/queue-*" || {
    echo "kubectl debug failed; ensure EphemeralContainers are enabled and cluster supports kubectl debug."
  }
done

