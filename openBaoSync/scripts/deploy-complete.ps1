#!/usr/bin/env pwsh

$ErrorActionPreference = "Stop"

Write-Host "=== OpenBao Certificate Sync - Complete Deployment ===" -ForegroundColor Green
Write-Host "This script will deploy everything needed for certificate sync`n" -ForegroundColor Cyan

Write-Host "Step 1: Deploying OpenBao..." -ForegroundColor Yellow
kubectl apply -f openBaoSync/kubectl/openbao-deployment.yaml
if ($LASTEXITCODE -ne 0) {
    Write-Host "Error: Failed to deploy OpenBao" -ForegroundColor Red
    exit 1
}

Write-Host "Waiting for OpenBao to be ready..." -ForegroundColor Gray
kubectl wait --for=condition=ready pod -l app=openbao -n openbao --timeout=120s
if ($LASTEXITCODE -ne 0) {
    Write-Host "Error: OpenBao failed to start" -ForegroundColor Red
    exit 1
}
Write-Host "✓ OpenBao is running" -ForegroundColor Green

Write-Host "`nStep 2: Generating and storing certificates..." -ForegroundColor Yellow
powershell -ExecutionPolicy Bypass -File openBaoSync/scripts/setup-openbao-certs.ps1
if ($LASTEXITCODE -ne 0) {
    Write-Host "Error: Failed to generate certificates" -ForegroundColor Red
    exit 1
}
Write-Host "✓ Certificates generated and stored" -ForegroundColor Green

Write-Host "`nStep 3: Installing CSI drivers..." -ForegroundColor Yellow
powershell -ExecutionPolicy Bypass -File openBaoSync/scripts/setup-openbao-csi.ps1
if ($LASTEXITCODE -ne 0) {
    Write-Host "Error: Failed to install CSI drivers" -ForegroundColor Red
    exit 1
}
Write-Host "✓ CSI drivers installed" -ForegroundColor Green

Write-Host "`nStep 4: Setting up Kubernetes authentication..." -ForegroundColor Yellow
$podName = kubectl get pods -n openbao -l app=openbao -o jsonpath='{.items[0].metadata.name}'
if (-not $podName) {
    Write-Host "Error: OpenBao pod not found" -ForegroundColor Red
    exit 1
}

$logs = kubectl logs -n openbao $podName
$tokenMatch = $logs | Select-String -Pattern "Root Token:\s+(.+)" | ForEach-Object { $_.Matches.Groups[1].Value }
if (-not $tokenMatch) {
    Write-Host "Error: Could not find root token in OpenBao logs" -ForegroundColor Red
    exit 1
}
$OPENBAO_TOKEN = $tokenMatch.Trim()

Write-Host "  Enabling Kubernetes auth method..." -ForegroundColor Gray
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao auth enable kubernetes" 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host "  (Kubernetes auth may already be enabled)" -ForegroundColor Gray
}

Write-Host "  Configuring Kubernetes auth..." -ForegroundColor Gray
$K8S_HOST = "https://kubernetes.default.svc"
$saToken = kubectl exec -n openbao $podName -- cat /var/run/secrets/kubernetes.io/serviceaccount/token
kubectl exec -n openbao $podName -- sh -c "cat /var/run/secrets/kubernetes.io/serviceaccount/ca.crt > /tmp/k8s-ca.crt"
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao write auth/kubernetes/config token_reviewer_jwt='$saToken' kubernetes_host='$K8S_HOST' kubernetes_ca_cert=@/tmp/k8s-ca.crt disable_iss_validation=true"
if ($LASTEXITCODE -ne 0) {
    Write-Host "Error: Failed to configure Kubernetes auth" -ForegroundColor Red
    exit 1
}

Write-Host "  Creating policy..." -ForegroundColor Gray
$policyBase64 = "cGF0aCAiY2VydHMvZGF0YS8qIiB7CiAgY2FwYWJpbGl0aWVzID0gWyJyZWFkIl0KfQ=="
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && echo '$policyBase64' | base64 -d | bao policy write otelcol1-policy -" 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host "  (Policy may already exist)" -ForegroundColor Gray
}

Write-Host "  Creating role..." -ForegroundColor Gray
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao write auth/kubernetes/role/otelcol1-role bound_service_account_names=otelcol1 bound_service_account_namespaces=otel-demo policies=otelcol1-policy ttl=1h" 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host "  (Role may already exist)" -ForegroundColor Gray
}
Write-Host "✓ Kubernetes authentication configured" -ForegroundColor Green

Write-Host "`nStep 5: Applying SecretProviderClass..." -ForegroundColor Yellow
kubectl apply -f openBaoSync/kubectl/openbao-csi-secretproviderclass.yaml
if ($LASTEXITCODE -ne 0) {
    Write-Host "Error: Failed to apply SecretProviderClass" -ForegroundColor Red
    exit 1
}
Write-Host "✓ SecretProviderClass applied" -ForegroundColor Green

Write-Host "`nStep 6: Deploying collector..." -ForegroundColor Yellow
kubectl apply -f openBaoSync/kubectl/otelcol1-with-csi.yaml
if ($LASTEXITCODE -ne 0) {
    Write-Host "Error: Failed to deploy collector" -ForegroundColor Red
    exit 1
}

Write-Host "Waiting for deployment to be ready..." -ForegroundColor Gray
kubectl rollout status deployment/otelcol1 -n otel-demo --timeout=120s
if ($LASTEXITCODE -ne 0) {
    Write-Host "Warning: Deployment may still be starting" -ForegroundColor Yellow
} else {
    Write-Host "✓ Collector deployed and ready" -ForegroundColor Green
}

Write-Host "`n=== Deployment Complete ===" -ForegroundColor Green
Write-Host "`nVerifying deployment..." -ForegroundColor Yellow
Start-Sleep -Seconds 5
powershell -ExecutionPolicy Bypass -File openBaoSync/scripts/proof-certificate-sync.ps1

Write-Host "`n=== Next Steps ===" -ForegroundColor Cyan
Write-Host "1. Verify certificates are accessible in the pod" -ForegroundColor White
Write-Host "2. Configure your application to use certificates from /mnt/secrets-store/" -ForegroundColor White
Write-Host "3. Monitor pod logs for any issues" -ForegroundColor White
Write-Host "`nFor detailed verification steps, see PROOF-CERTIFICATE-SYNC.md" -ForegroundColor Gray
