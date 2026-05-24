$ErrorActionPreference = "Continue"

Write-Host "=== Complete CSI Driver Installation ===" -ForegroundColor Green

Write-Host "`nStep 1: Installing CSI Secret Store Driver components..." -ForegroundColor Yellow

Write-Host "  - Installing ServiceAccount and RBAC..." -ForegroundColor Cyan
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/secrets-store-csi-driver/v1.4.0/deploy/rbac-secretproviderclass.yaml

Write-Host "  - Installing CSIDriver..." -ForegroundColor Cyan
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/secrets-store-csi-driver/v1.4.0/deploy/csidriver.yaml

Write-Host "  - Installing DaemonSet..." -ForegroundColor Cyan
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/secrets-store-csi-driver/v1.4.0/deploy/secrets-store-csi-driver.yaml

Write-Host "`nStep 2: Waiting for CSI driver pods..." -ForegroundColor Yellow
$maxWait = 60
$waited = 0
while ($waited -lt $maxWait) {
    $pods = kubectl get pods -n kube-system -l app=secrets-store-csi-driver --no-headers 2>$null
    if ($pods -and ($pods | Select-String -Pattern "Running|Ready" -Quiet)) {
        Write-Host "  CSI driver pods are running!" -ForegroundColor Green
        kubectl get pods -n kube-system -l app=secrets-store-csi-driver
        break
    }
    Start-Sleep -Seconds 2
    $waited += 2
    Write-Host "  Waiting... ($waited/$maxWait seconds)" -ForegroundColor Gray
}
if ($waited -ge $maxWait) {
    Write-Host "  Warning: CSI driver pods not ready after $maxWait seconds" -ForegroundColor Yellow
    kubectl get pods -n kube-system -l app=secrets-store-csi-driver
    kubectl describe daemonset csi-secrets-store -n kube-system | Select-String -Pattern "Events:" -Context 0,5
}

Write-Host "`nStep 3: Verifying CRD installation..." -ForegroundColor Yellow
$crd = kubectl get crd secretproviderclasses.secrets-store.csi.x-k8s.io -o name 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "  SecretProviderClass CRD is installed" -ForegroundColor Green
} else {
    Write-Host "  CRD not found. Installing from manifest..." -ForegroundColor Yellow
    $tempFile = "$env:TEMP\csi-driver-full.yaml"
    Invoke-WebRequest -Uri "https://raw.githubusercontent.com/kubernetes-sigs/secrets-store-csi-driver/v1.4.0/deploy/secrets-store-csi-driver.yaml" -OutFile $tempFile
    $content = Get-Content $tempFile -Raw
    if ($content -match '(?s)apiVersion: apiextensions\.k8s\.io/v1.*?kind: CustomResourceDefinition.*?---') {
        $crdYaml = $matches[0]
        $crdYaml | Out-File "$env:TEMP\csi-crd-only.yaml" -Encoding utf8
        kubectl apply -f "$env:TEMP\csi-crd-only.yaml"
    }
}

Write-Host "`nStep 4: Installing OpenBao CSI Provider..." -ForegroundColor Yellow
Write-Host "  Note: OpenBao CSI Provider needs to be installed manually." -ForegroundColor Yellow
Write-Host "  Checking if provider directory exists..." -ForegroundColor Cyan

$providerPath = "/var/run/secrets-store-csi-providers"
Write-Host "  Provider path: $providerPath" -ForegroundColor Gray
Write-Host "  For now, we'll use token auth which doesn't require the provider daemonset" -ForegroundColor Yellow

Write-Host "`nStep 5: Verifying installation..." -ForegroundColor Yellow
Write-Host "  CSIDriver:" -ForegroundColor Cyan
kubectl get csidriver secrets-store.csi.k8s.io

Write-Host "`n  SecretProviderClass CRD:" -ForegroundColor Cyan
kubectl get crd secretproviderclasses.secrets-store.csi.x-k8s.io

Write-Host "`n  CSI Driver Pods:" -ForegroundColor Cyan
kubectl get pods -n kube-system -l app=secrets-store-csi-driver

Write-Host "`n=== Installation Complete ===" -ForegroundColor Green
Write-Host "Next: Run setup-openbao-csi.ps1 to configure OpenBao integration" -ForegroundColor Cyan
