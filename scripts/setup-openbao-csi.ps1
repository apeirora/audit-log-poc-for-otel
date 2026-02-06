$ErrorActionPreference = "Continue"

Write-Host "Setting up OpenBao CSI Provider integration..." -ForegroundColor Green

Write-Host "`nStep 1: Checking if CSI Secret Store Driver is installed..." -ForegroundColor Yellow
try {
    $null = kubectl get csidriver secrets-store.csi.k8s.io -o name 2>&1
    if ($LASTEXITCODE -eq 0) {
        Write-Host "CSI Secret Store Driver is already installed" -ForegroundColor Green
    } else {
        throw "Not found"
    }
} catch {
    Write-Host "CSI Secret Store Driver not found. Installing..." -ForegroundColor Yellow
    kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/secrets-store-csi-driver/v1.4.0/deploy/secrets-store-csi-driver.yaml
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Waiting for CSI driver to be ready..." -ForegroundColor Yellow
        kubectl wait --for=condition=ready pod -l app=secrets-store-csi-driver -n kube-system --timeout=120s
    }
}

Write-Host "`nStep 2: Checking if OpenBao CSI Provider is installed..." -ForegroundColor Yellow
try {
    $null = kubectl get daemonset -n kube-system -l app=openbao-csi-provider -o name 2>&1
    if ($LASTEXITCODE -eq 0) {
        Write-Host "OpenBao CSI Provider is already installed" -ForegroundColor Green
    } else {
        throw "Not found"
    }
} catch {
    Write-Host "OpenBao CSI Provider not found. Installing..." -ForegroundColor Yellow
    kubectl apply -f https://raw.githubusercontent.com/openbao/openbao-csi-provider/main/deploy/install.yaml
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Waiting for CSI provider to be ready..." -ForegroundColor Yellow
        kubectl wait --for=condition=ready pod -l app=openbao-csi-provider -n kube-system --timeout=120s
    }
}

Write-Host "`nStep 3: Extracting OpenBao root token..." -ForegroundColor Yellow
$OPENBAO_NAMESPACE = "openbao"
$podName = kubectl get pods -n $OPENBAO_NAMESPACE -l app=openbao -o jsonpath='{.items[0].metadata.name}'
if (-not $podName) {
    Write-Host "Error: OpenBao pod not found. Please deploy OpenBao first." -ForegroundColor Red
    exit 1
}

$logs = kubectl logs -n $OPENBAO_NAMESPACE $podName
$tokenMatch = $logs | Select-String -Pattern "Root Token:\s+(.+)" | ForEach-Object { $_.Matches.Groups[1].Value }
if (-not $tokenMatch) {
    Write-Host "Error: Could not find root token in OpenBao logs" -ForegroundColor Red
    exit 1
}
$OPENBAO_TOKEN = $tokenMatch.Trim()
Write-Host "Found root token" -ForegroundColor Green

Write-Host "`nStep 4: Creating OpenBao token secret..." -ForegroundColor Yellow
$tokenSecret = @"
apiVersion: v1
kind: Secret
metadata:
  name: openbao-token
  namespace: otel-demo
type: Opaque
stringData:
  token: "$OPENBAO_TOKEN"
"@
$tokenSecret | kubectl apply -f -

Write-Host "`nStep 5: Applying RBAC configuration..." -ForegroundColor Yellow
kubectl apply -f kubectl/openbao-csi-rbac.yaml

Write-Host "`nStep 6: Applying SecretProviderClass..." -ForegroundColor Yellow
kubectl apply -f kubectl/openbao-csi-secretproviderclass.yaml

Write-Host "`nStep 7: Updating otelcol1 deployment with CSI volumes..." -ForegroundColor Yellow
kubectl apply -f kubectl/otelcol1-with-csi.yaml

Write-Host "`nWaiting for deployment to be ready..." -ForegroundColor Yellow
kubectl rollout status deployment/otelcol1 -n otel-demo --timeout=120s

Write-Host "`nStep 8: Verifying CSI integration..." -ForegroundColor Yellow
Start-Sleep -Seconds 5
$secret = kubectl get secret otelcol1-certs -n otel-demo -o name 2>$null
if ($secret) {
    Write-Host "Success! Secret 'otelcol1-certs' was created by CSI provider" -ForegroundColor Green
    kubectl get secret otelcol1-certs -n otel-demo -o jsonpath='{.data}' | ConvertFrom-Json | Get-Member -MemberType NoteProperty | ForEach-Object { Write-Host "  - $($_.Name)" }
} else {
    Write-Host "Warning: Secret not yet created. It will be created when the pod starts." -ForegroundColor Yellow
}

Write-Host "`nDone! OpenBao CSI Provider integration is complete." -ForegroundColor Green
Write-Host "`nCertificates are now automatically mounted via CSI volumes:" -ForegroundColor Cyan
Write-Host "  - File mount: /mnt/secrets-store/" -ForegroundColor Gray
Write-Host "  - K8s Secret: otelcol1-certs (synced automatically)" -ForegroundColor Gray
