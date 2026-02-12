# Persistent File Queue Multi Collector (K8s quick guide)

Namespace: `otel-demo`

## Deploy core resources

```bash
kubectl apply -f kubernetes.yaml
```

## Check pods and logs

- Pods: `kubectl get pods -n otel-demo`
- Storage `kubectl get pvc -n otel-demo`
- Collector logs (example): `kubectl logs deploy/otelcol1 -n otel-demo --tail=100`

## Send test logs from inside the cluster

```bash
kubectl apply -f send-logs-job.yaml
kubectl wait --for=condition=complete job/send-logs -n otel-demo --timeout=60s
kubectl logs job/send-logs -n otel-demo
kubectl delete job/send-logs -n otel-demo
```

## Send test logs from local machine (port-forward)

```bash
bash send-logs-k8.sh
```

- Set `KUBECONFIG`/`KCTX` envs if your kubeconfig/context is not the default.

## Inspect queue storage (shared PVC)

```bash
kubectl apply -f storage-check.yaml
kubectl wait --for=condition=complete job/storage-check -n otel-demo --timeout=60s
kubectl logs job/storage-check -n otel-demo
kubectl delete job/storage-check -n otel-demo
```

## Clear all queues (reset storage)

```bash
kubectl scale deploy/otelcol1 deploy/otelcol2 deploy/otelcol3 -n otel-demo --replicas=0
kubectl delete pvc storage-data -n otel-demo
kubectl apply -f kubernetes.yaml
kubectl scale deploy/otelcol1 deploy/otelcol2 deploy/otelcol3 -n otel-demo --replicas=1
```

## Clean up everything

```bash
kubectl delete namespace otel-demo
```
