#!/usr/bin/env pwsh

$ErrorActionPreference = "Continue"

$OPENBAO_NAMESPACE = "openbao"
$OPENBAO_SERVICE = "openbao"
$OPENBAO_ADDR = "http://openbao.openbao.svc.cluster.local:8200"
# COMMENTED OUT: Kubernetes secret creation - testing if CSI can fetch directly from OpenBao
# $SECRET_NAME = "openbao-certs"
# $SECRET_NAMESPACE = "default"

Write-Host "Waiting for OpenBao to be ready..." -ForegroundColor Green
kubectl wait --for=condition=ready pod -l app=openbao -n $OPENBAO_NAMESPACE --timeout=120s

Write-Host "Extracting root token from OpenBao logs..." -ForegroundColor Yellow
$podName = kubectl get pods -n $OPENBAO_NAMESPACE -l app=openbao -o jsonpath='{.items[0].metadata.name}'
$logs = kubectl logs -n $OPENBAO_NAMESPACE $podName
$tokenMatch = $logs | Select-String -Pattern "Root Token:\s+(.+)" | ForEach-Object { $_.Matches.Groups[1].Value }
if (-not $tokenMatch) {
    Write-Host "Error: Could not find root token in OpenBao logs" -ForegroundColor Red
    exit 1
}
$OPENBAO_TOKEN = $tokenMatch.Trim()
Write-Host "Found root token: $OPENBAO_TOKEN" -ForegroundColor Green

Write-Host "Setting up OpenBao PKI..." -ForegroundColor Green

Write-Host "Enabling PKI secrets engine..." -ForegroundColor Yellow
$pkiOutput = kubectl exec -n $OPENBAO_NAMESPACE $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao secrets enable pki 2>&1" 2>&1
if ($pkiOutput -match "already in use|Success") {
    if ($pkiOutput -match "already in use") {
        Write-Host "  PKI secrets engine already enabled" -ForegroundColor Gray
    }
} else {
    Write-Host "  Note: $($pkiOutput -join ' ')" -ForegroundColor Yellow
}

Write-Host "Setting max TTL for PKI..." -ForegroundColor Yellow
kubectl exec -n $OPENBAO_NAMESPACE $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao secrets tune -max-lease-ttl=87600h pki" 2>$null

Write-Host "Checking if root CA exists..." -ForegroundColor Yellow
$caCheck = kubectl exec -n $OPENBAO_NAMESPACE $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao read -format=json pki/cert/ca 2>&1" 2>&1
if ($caCheck -match '"certificate"') {
    Write-Host "  Root CA already exists, using existing CA" -ForegroundColor Gray
    $rootCaJson = $caCheck | Select-String -Pattern '\{.*\}' | ForEach-Object { $_.Matches.Value }
    $rootCaResult = $rootCaJson | ConvertFrom-Json
} else {
    Write-Host "Generating root CA..." -ForegroundColor Yellow
    $rootCaJson = kubectl exec -n $OPENBAO_NAMESPACE $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao write -format=json pki/root/generate/internal common_name='Test Root CA' ttl=87600h"
    if ($rootCaJson) {
        $rootCaResult = $rootCaJson | ConvertFrom-Json
    } else {
        Write-Host "  Warning: Could not generate root CA, may already exist" -ForegroundColor Yellow
        $rootCaJson = kubectl exec -n $OPENBAO_NAMESPACE $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao read -format=json pki/cert/ca"
        $rootCaResult = $rootCaJson | ConvertFrom-Json
    }
}

Write-Host "Configuring CA and CRL URLs..." -ForegroundColor Yellow
kubectl exec -n $OPENBAO_NAMESPACE $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao write pki/config/urls issuing_certificates='$OPENBAO_ADDR/v1/pki/ca' crl_distribution_points='$OPENBAO_ADDR/v1/pki/crl'"

Write-Host "Enabling KV secrets engine for storing certificates..." -ForegroundColor Yellow
$kvOutput = kubectl exec -n $OPENBAO_NAMESPACE $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao secrets enable -path=certs kv-v2 2>&1" 2>&1
if ($kvOutput -match "already in use|Success") {
    if ($kvOutput -match "already in use") {
        Write-Host "  KV secrets engine at certs/ already enabled" -ForegroundColor Gray
    }
} else {
    Write-Host "  Note: $($kvOutput -join ' ')" -ForegroundColor Yellow
}

Write-Host "Creating test role for certificates..." -ForegroundColor Yellow
$roleOutput = kubectl exec -n $OPENBAO_NAMESPACE $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao write pki/roles/test-role allowed_domains='test.local' allow_subdomains=true max_ttl=72h 2>&1" 2>&1
if ($roleOutput -match "Success") {
    Write-Host "  Test role created/updated" -ForegroundColor Gray
} else {
    Write-Host "  Test role may already exist, continuing..." -ForegroundColor Gray
}

Write-Host "Generating test certificate 1..." -ForegroundColor Yellow
$cert1Json = kubectl exec -n $OPENBAO_NAMESPACE $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao write -format=json pki/issue/test-role common_name='test1.test.local' ttl=24h"
$cert1Result = $cert1Json | ConvertFrom-Json

Write-Host "Storing certificate 1 in OpenBao KV store..." -ForegroundColor Yellow
$cert1CaChain = if ($cert1Result.data.ca_chain -is [array]) { $cert1Result.data.ca_chain -join "`n" } else { $cert1Result.data.ca_chain }
$cert1DataJson = @{
    data = @{
        certificate = $cert1Result.data.certificate
        private_key = $cert1Result.data.private_key
        ca_chain = $cert1CaChain
    }
} | ConvertTo-Json -Depth 3
$cert1DataJsonBase64 = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($cert1DataJson))
kubectl exec -n $OPENBAO_NAMESPACE $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && echo '$cert1DataJsonBase64' | base64 -d > /tmp/cert1.json && bao write certs/data/test1 @/tmp/cert1.json && rm /tmp/cert1.json"

Write-Host "Generating test certificate 2..." -ForegroundColor Yellow
$cert2Json = kubectl exec -n $OPENBAO_NAMESPACE $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao write -format=json pki/issue/test-role common_name='test2.test.local' ttl=24h"
$cert2Result = $cert2Json | ConvertFrom-Json

Write-Host "Storing certificate 2 in OpenBao KV store..." -ForegroundColor Yellow
$cert2CaChain = if ($cert2Result.data.ca_chain -is [array]) { $cert2Result.data.ca_chain -join "`n" } else { $cert2Result.data.ca_chain }
$cert2DataJson = @{
    data = @{
        certificate = $cert2Result.data.certificate
        private_key = $cert2Result.data.private_key
        ca_chain = $cert2CaChain
    }
} | ConvertTo-Json -Depth 3
$cert2DataJsonBase64 = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($cert2DataJson))
kubectl exec -n $OPENBAO_NAMESPACE $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && echo '$cert2DataJsonBase64' | base64 -d > /tmp/cert2.json && bao write certs/data/test2 @/tmp/cert2.json && rm /tmp/cert2.json"

Write-Host "`nSuccess! Certificates have been stored in OpenBao KV store at certs/data/test1 and certs/data/test2" -ForegroundColor Green
Write-Host "CSI provider will fetch certificates directly from OpenBao when pods start." -ForegroundColor Cyan

# COMMENTED OUT: Kubernetes secret creation - testing if CSI can fetch directly from OpenBao
# Write-Host "Getting root CA certificate..." -ForegroundColor Yellow
# $rootCaCertJson = kubectl exec -n $OPENBAO_NAMESPACE $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao read -format=json pki/cert/ca"
# $rootCaCert = $rootCaCertJson | ConvertFrom-Json
#
# Write-Host "Preparing certificate data for Kubernetes secret..." -ForegroundColor Yellow
#
# $cert1Cert = $cert1Result.data.certificate
# $cert1Key = $cert1Result.data.private_key
# if ($cert1Result.data.ca_chain) {
#     $cert1CaChain = if ($cert1Result.data.ca_chain -is [array]) { $cert1Result.data.ca_chain -join "`n" } else { $cert1Result.data.ca_chain }
# } else {
#     $cert1CaChain = ""
# }
#
# $cert2Cert = $cert2Result.data.certificate
# $cert2Key = $cert2Result.data.private_key
# if ($cert2Result.data.ca_chain) {
#     $cert2CaChain = if ($cert2Result.data.ca_chain -is [array]) { $cert2Result.data.ca_chain -join "`n" } else { $cert2Result.data.ca_chain }
# } else {
#     $cert2CaChain = ""
# }
#
# $rootCaCertData = $rootCaCert.data.certificate
#
# $tempDir = New-TemporaryFile | ForEach-Object { Remove-Item $_; New-Item -ItemType Directory -Path $_ }
# $cert1CertFile = Join-Path $tempDir "cert1.crt"
# $cert1KeyFile = Join-Path $tempDir "cert1.key"
# $cert1CaFile = Join-Path $tempDir "cert1-ca.crt"
# $cert2CertFile = Join-Path $tempDir "cert2.crt"
# $cert2KeyFile = Join-Path $tempDir "cert2.key"
# $cert2CaFile = Join-Path $tempDir "cert2-ca.crt"
# $rootCaFile = Join-Path $tempDir "root-ca.crt"
#
# Set-Content -Path $cert1CertFile -Value $cert1Cert
# Set-Content -Path $cert1KeyFile -Value $cert1Key
# Set-Content -Path $cert1CaFile -Value $cert1CaChain
# Set-Content -Path $cert2CertFile -Value $cert2Cert
# Set-Content -Path $cert2KeyFile -Value $cert2Key
# Set-Content -Path $cert2CaFile -Value $cert2CaChain
# Set-Content -Path $rootCaFile -Value $rootCaCertData
#
# Write-Host "Creating Kubernetes secret with certificates..." -ForegroundColor Green
# kubectl create secret generic $SECRET_NAME `
#     --from-file=cert1.crt=$cert1CertFile `
#     --from-file=cert1.key=$cert1KeyFile `
#     --from-file=cert1-ca.crt=$cert1CaFile `
#     --from-file=cert2.crt=$cert2CertFile `
#     --from-file=cert2.key=$cert2KeyFile `
#     --from-file=cert2-ca.crt=$cert2CaFile `
#     --from-file=root-ca.crt=$rootCaFile `
#     --namespace=$SECRET_NAMESPACE `
#     --dry-run=client -o yaml | kubectl apply -f -
#
# Write-Host "Cleaning up temporary files..." -ForegroundColor Yellow
# Remove-Item -Recurse -Force $tempDir
#
# Write-Host "`nSuccess! Certificates have been stored in Kubernetes secret '$SECRET_NAME' in namespace '$SECRET_NAMESPACE'" -ForegroundColor Green
# Write-Host "`nSecret contents:" -ForegroundColor Cyan
# kubectl get secret $SECRET_NAME -n $SECRET_NAMESPACE -o jsonpath='{.data}' | ConvertFrom-Json | Get-Member -MemberType NoteProperty | ForEach-Object { Write-Host "  - $($_.Name)" }
#
# Write-Host "`nTo view a certificate, use:" -ForegroundColor Cyan
# Write-Host "  kubectl get secret $SECRET_NAME -n $SECRET_NAMESPACE -o jsonpath='{.data.cert1\.crt}' | ForEach-Object { [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String(`$_)) }" -ForegroundColor Gray
