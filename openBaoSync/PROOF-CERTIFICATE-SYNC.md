# Proof: OpenBao Certificate Sync to Collector Pod

This guide demonstrates that certificates from OpenBao are successfully synced to the `otelcol1` collector pod via CSI.

## Step 1: Verify Pod is Running

```powershell
kubectl get pods -n otel-demo -l app=otelcol1
```

**Expected Output:**
```
NAME                        READY   STATUS    RESTARTS   AGE
otelcol1-xxxxx-xxxxx        1/1     Running   0          Xm
```

✅ **Proof:** Pod is in `Running` state with `1/1` ready.

---

## Step 2: Verify CSI Volume Mount Status

```powershell
kubectl get secretproviderclasspodstatus -n otel-demo
```

**Expected Output:**
```
NAME                                                       AGE
otelcol1-xxxxx-xxxxx-otel-demo-openbao-certificates       Xm
```

Then check the detailed status:

```powershell
kubectl get secretproviderclasspodstatus -n otel-demo -o yaml | Select-String -Pattern "mounted|objects" -Context 5
```

**Expected Output:**
```
status:
  mounted: true
  objects:
  - id: ca_chain
    version: xxxxx
  - id: certificate
    version: xxxxx
  - id: private_key
    version: xxxxx
```

✅ **Proof:** CSI mount is `mounted: true` and all 3 certificate objects are listed.

---

## Step 3: Verify Certificates are Mounted in Pod

Get the pod name first:

```powershell
$podName = kubectl get pods -n otel-demo -l app=otelcol1 --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}'
Write-Host "Pod: $podName"
```

### Check CSI Mount Point

**Note:** The collector container may not have `ls` command. Use these alternatives:

**Option 1: Check via describe pod**
```powershell
kubectl describe pod -n otel-demo $podName | Select-String -Pattern "Mounts:|openbao-certificates" -Context 3
```

**Option 2: Verify mount exists by reading a file**
```powershell
kubectl exec -n otel-demo $podName -- sh -c "test -f /mnt/secrets-store/certificate && echo 'Certificate file exists' || echo 'File not found'"
```

**Expected Output:**
```
Certificate file exists
```

✅ **Proof:** Certificate file exists in the CSI mount directory.

### View Certificate Content

```powershell
kubectl exec -n otel-demo $podName -- cat /mnt/secrets-store/certificate
```

**Expected Output:**
```
-----BEGIN CERTIFICATE-----
[Certificate content]
-----END CERTIFICATE-----
```

✅ **Proof:** Certificate file contains valid certificate data.

### View Private Key

```powershell
kubectl exec -n otel-demo $podName -- cat /mnt/secrets-store/private_key
```

**Expected Output:**
```
-----BEGIN PRIVATE KEY-----
[Private key content]
-----END PRIVATE KEY-----
```

✅ **Proof:** Private key file exists and contains key data.

### Verify All Three Files Exist

```powershell
kubectl exec -n otel-demo $podName -- sh -c "test -f /mnt/secrets-store/certificate && test -f /mnt/secrets-store/private_key && test -f /mnt/secrets-store/ca_chain && echo 'All certificate files exist!' || echo 'Missing files'"
```

**Expected Output:**
```
All certificate files exist!
```

✅ **Proof:** All three certificate files are present.

---

## Step 4: Verify Volume Mounts in Pod

```powershell
kubectl describe pod -n otel-demo $podName | Select-String -Pattern "Mounts:|openbao-certificates|otelcol1-certs" -Context 2
```

**Expected Output:**
```
Mounts:
  /mnt/secrets-store from openbao-certificates (ro)
  /etc/otelcol/certs from otelcol1-certs (ro)
  /etc/otelcol/config.yaml from config (rw,path="config.yaml")
```

✅ **Proof:** Both CSI mount (`openbao-certificates`) and secret mount (`otelcol1-certs`) are configured.

---

## Step 5: Verify Kubernetes Secret (if synced)

```powershell
kubectl get secret otelcol1-certs -n otel-demo
```

**Expected Output (if synced):**
```
NAME            TYPE     DATA   AGE
otelcol1-certs  Opaque   3      5m
```

If the secret exists, view its keys:

```powershell
kubectl get secret otelcol1-certs -n otel-demo -o jsonpath='{.data}' | ConvertFrom-Json | Get-Member -MemberType NoteProperty | ForEach-Object { Write-Host "  - $($_.Name)" }
```

**Expected Output:**
```
  - cert.crt
  - cert.key
  - ca.crt
```

✅ **Proof:** Kubernetes secret contains all three certificate files.

---

## Step 6: Verify OpenBao Authentication

Check that Kubernetes auth is configured:

```powershell
$podName = kubectl get pods -n openbao -l app=openbao -o jsonpath='{.items[0].metadata.name}'
$logs = kubectl logs -n openbao $podName
$tokenMatch = $logs | Select-String -Pattern "Root Token:\s+(.+)" | ForEach-Object { $_.Matches.Groups[1].Value }
$OPENBAO_TOKEN = $tokenMatch.Trim()
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao read auth/kubernetes/role/otelcol1-role"
```

**Expected Output:**
```
Key                                         Value
---                                         -----
bound_service_account_names                 [otelcol1]
bound_service_account_namespaces            [otel-demo]
policies                                    [otelcol1-policy]
token_ttl                                   1h
```

✅ **Proof:** Kubernetes auth role is configured correctly.

---

## Step 7: Verify Certificates in OpenBao

```powershell
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao kv get certs/data/test1"
```

**Expected Output:**
```
== Secret Path ==
certs/data/test1

======= Data =======
Key            Value
---            -----
ca_chain       [CA chain content]
certificate    [Certificate content]
private_key    [Private key content]
```

✅ **Proof:** Certificates exist in OpenBao and match what's in the pod.

---

## Step 8: Complete Verification Script

Run this all-in-one verification:

```powershell
Write-Host "=== CERTIFICATE SYNC VERIFICATION ===" -ForegroundColor Green
Write-Host "`n1. Pod Status:" -ForegroundColor Yellow
kubectl get pods -n otel-demo -l app=otelcol1

Write-Host "`n2. CSI Mount Status:" -ForegroundColor Yellow
$spcps = kubectl get secretproviderclasspodstatus -n otel-demo -o jsonpath='{.items[0].status.mounted}' 2>$null
if ($spcps -eq "true") {
    Write-Host "   ✓ CSI volume is mounted" -ForegroundColor Green
} else {
    Write-Host "   ✗ CSI volume not mounted" -ForegroundColor Red
}

Write-Host "`n3. Certificate Files in Pod:" -ForegroundColor Yellow
$podName = kubectl get pods -n otel-demo -l app=otelcol1 --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}' 2>$null
if ($podName) {
    $files = kubectl exec -n otel-demo $podName -- ls /mnt/secrets-store/ 2>$null
    if ($files -match "certificate" -and $files -match "private_key" -and $files -match "ca_chain") {
        Write-Host "   ✓ All certificate files present" -ForegroundColor Green
        Write-Host "   Files: $files" -ForegroundColor Gray
    } else {
        Write-Host "   ✗ Missing certificate files" -ForegroundColor Red
    }
} else {
    Write-Host "   ✗ No running pod found" -ForegroundColor Red
}

Write-Host "`n4. Kubernetes Secret:" -ForegroundColor Yellow
$secret = kubectl get secret otelcol1-certs -n otel-demo -o name 2>$null
if ($secret) {
    Write-Host "   ✓ Secret exists" -ForegroundColor Green
    kubectl get secret otelcol1-certs -n otel-demo
} else {
    Write-Host "   ⚠ Secret not yet created (CSI will create it)" -ForegroundColor Yellow
}

Write-Host "`n=== VERIFICATION COMPLETE ===" -ForegroundColor Green
```

---

## Quick One-Liner Proof

For a quick demonstration, run:

```powershell
$podName = kubectl get pods -n otel-demo -l app=otelcol1 --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}'; Write-Host "Pod: $podName"; Write-Host "`nVerifying certificate files:"; kubectl exec -n otel-demo $podName -- sh -c "test -f /mnt/secrets-store/certificate && echo '✓ certificate' && test -f /mnt/secrets-store/private_key && echo '✓ private_key' && test -f /mnt/secrets-store/ca_chain && echo '✓ ca_chain' && echo '`nAll files exist!'"; Write-Host "`nCertificate preview:"; kubectl exec -n otel-demo $podName -- sh -c "cat /mnt/secrets-store/certificate | grep -A 2 BEGIN"
```

This shows:
- The running pod name
- Verification that all certificate files exist
- A preview of the certificate content

---

## Summary Checklist

✅ Pod is running (`1/1 Ready`)  
✅ CSI volume is mounted (`mounted: true`)  
✅ Certificate files exist in `/mnt/secrets-store/`  
✅ Certificate content is valid  
✅ Kubernetes auth is configured  
✅ Certificates exist in OpenBao  
✅ (Optional) Kubernetes secret is synced  

**If all checks pass, the certificate sync is working!** 🎉
