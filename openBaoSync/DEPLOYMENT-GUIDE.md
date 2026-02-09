# Deployment Guide: OpenBao Certificate Sync to Kubernetes Cluster

This guide walks you through deploying the complete OpenBao certificate management solution to your Kubernetes cluster.

## Prerequisites

Before starting, ensure you have:

- ✅ Kubernetes cluster running and accessible
- ✅ `kubectl` configured and connected to your cluster
- ✅ PowerShell (Windows) or Bash (Linux/Mac)
- ✅ Cluster admin permissions (for installing CSI drivers)

## Step 1: Verify Cluster Access

```powershell
kubectl cluster-info
kubectl get nodes
```

**Expected:** You should see your cluster information and nodes listed.

---

## Step 2: Deploy OpenBao

Deploy OpenBao to your cluster:

```powershell
kubectl apply -f kubectl/openbao-deployment.yaml
```

Wait for OpenBao to be ready:

```powershell
kubectl wait --for=condition=ready pod -l app=openbao -n openbao --timeout=120s
```

Verify it's running:

```powershell
kubectl get pods -n openbao -l app=openbao
```

**Expected Output:**
```
NAME                       READY   STATUS    RESTARTS   AGE
openbao-xxxxx-xxxxx        1/1     Running   0          Xm
```

---

## Step 3: Generate and Store Certificates

Run the certificate setup script:

**PowerShell (Windows):**
```powershell
powershell -ExecutionPolicy Bypass -File scripts/setup-openbao-certs.ps1
```

**Bash (Linux/Mac):**
```bash
bash scripts/setup-openbao-certs.sh
```

This script will:
- Extract root token from OpenBao logs
- Enable PKI secrets engine
- Generate root CA
- Create test certificates
- Store certificates in OpenBao KV store at `certs/data/test1` and `certs/data/test2`

**Verify certificates were created:**
```powershell
$podName = kubectl get pods -n openbao -l app=openbao -o jsonpath='{.items[0].metadata.name}'
$logs = kubectl logs -n openbao $podName
$tokenMatch = $logs | Select-String -Pattern "Root Token:\s+(.+)" | ForEach-Object { $_.Matches.Groups[1].Value }
$OPENBAO_TOKEN = $tokenMatch.Trim()
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao kv get certs/data/test1"
```

**Expected:** You should see certificate, private_key, and ca_chain data.

---

## Step 4: Install CSI Drivers

Install the CSI Secret Store Driver and OpenBao CSI Provider:

**PowerShell (Windows):**
```powershell
powershell -ExecutionPolicy Bypass -File scripts/setup-openbao-csi.ps1
```

**Bash (Linux/Mac):**
```bash
bash scripts/setup-openbao-csi.sh
```

This script will:
1. Install CSI Secret Store Driver (if not already installed)
2. Install OpenBao CSI Provider (if not already installed)
3. Create OpenBao token secret (for initial setup)
4. Apply RBAC configuration
5. Create SecretProviderClass
6. Update `otelcol1` deployment with CSI volumes

**Verify CSI drivers are running:**
```powershell
kubectl get pods -n kube-system -l app=secrets-store-csi-driver
kubectl get pods -n kube-system -l app=openbao-csi-provider
```

**Expected:** Both should show pods in `Running` state.

---

## Step 5: Setup Kubernetes Authentication in OpenBao

**Important:** This is required for the CSI provider to authenticate. The setup script uses token auth, but we configure Kubernetes auth for better security.

### 5.1: Get OpenBao Root Token

```powershell
$podName = kubectl get pods -n openbao -l app=openbao -o jsonpath='{.items[0].metadata.name}'
$logs = kubectl logs -n openbao $podName
$tokenMatch = $logs | Select-String -Pattern "Root Token:\s+(.+)" | ForEach-Object { $_.Matches.Groups[1].Value }
$OPENBAO_TOKEN = $tokenMatch.Trim()
Write-Host "Root token extracted: $($OPENBAO_TOKEN.Substring(0,10))..."
```

### 5.2: Enable Kubernetes Auth Method

```powershell
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao auth enable kubernetes"
```

**Expected Output:**
```
Success! Enabled kubernetes auth method at: kubernetes/
```

### 5.3: Configure Kubernetes Auth

```powershell
$K8S_HOST = "https://kubernetes.default.svc"
$saToken = kubectl exec -n openbao $podName -- cat /var/run/secrets/kubernetes.io/serviceaccount/token
kubectl exec -n openbao $podName -- sh -c "cat /var/run/secrets/kubernetes.io/serviceaccount/ca.crt > /tmp/k8s-ca.crt"
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao write auth/kubernetes/config token_reviewer_jwt='$saToken' kubernetes_host='$K8S_HOST' kubernetes_ca_cert=@/tmp/k8s-ca.crt disable_iss_validation=true"
```

**Expected Output:**
```
Success! Data written to: auth/kubernetes/config
```

### 5.4: Create Policy for Certificate Access

```powershell
$policyBase64 = "cGF0aCAiY2VydHMvZGF0YS8qIiB7CiAgY2FwYWJpbGl0aWVzID0gWyJyZWFkIl0KfQ=="
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && echo '$policyBase64' | base64 -d | bao policy write otelcol1-policy -"
```

**Expected Output:**
```
Success! Uploaded policy: otelcol1-policy
```

### 5.5: Create Role Mapping ServiceAccount to Policy

```powershell
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao write auth/kubernetes/role/otelcol1-role bound_service_account_names=otelcol1 bound_service_account_namespaces=otel-demo policies=otelcol1-policy ttl=1h"
```

**Expected Output:**
```
Success! Data written to: auth/kubernetes/role/otelcol1-role
```

### 5.6: Verify Kubernetes Auth Setup

```powershell
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao read auth/kubernetes/role/otelcol1-role"
```

**Expected:** Should show role configuration with bound service account and policies.

---

## Step 6: Apply Updated SecretProviderClass

The SecretProviderClass should already be configured for Kubernetes auth, but verify and apply:

```powershell
kubectl apply -f kubectl/openbao-csi-secretproviderclass.yaml
```

**Verify configuration:**
```powershell
kubectl get secretproviderclass openbao-certificates -n otel-demo -o jsonpath='{.spec.parameters.vaultAuthMethod}'
```

**Expected Output:**
```
kubernetes
```

---

## Step 7: Deploy Collector with CSI Volumes

Apply the deployment:

```powershell
kubectl apply -f kubectl/otelcol1-with-csi.yaml
```

Wait for deployment to be ready:

```powershell
kubectl rollout status deployment/otelcol1 -n otel-demo --timeout=120s
```

**Verify pod is running:**
```powershell
kubectl get pods -n otel-demo -l app=otelcol1
```

**Expected Output:**
```
NAME                        READY   STATUS    RESTARTS   AGE
otelcol1-xxxxx-xxxxx        1/1     Running   0          Xm
```

---

## Step 8: Verify Everything Works

### 8.1: Check Pod Status

```powershell
kubectl get pods -n otel-demo -l app=otelcol1
```

✅ Pod should be `1/1 Running`

### 8.2: Check CSI Mount Status

```powershell
kubectl get secretproviderclasspodstatus -n otel-demo
```

Then check detailed status:

```powershell
kubectl get secretproviderclasspodstatus -n otel-demo -o yaml | Select-String -Pattern "mounted|objects" -Context 3
```

✅ Should show `mounted: true` and list all 3 certificate objects

### 8.3: Verify Certificates in Pod

```powershell
$podName = kubectl get pods -n otel-demo -l app=otelcol1 --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}'
kubectl exec -n otel-demo $podName -- sh -c "test -f /mnt/secrets-store/certificate && echo '✓ Certificate exists'"
kubectl exec -n otel-demo $podName -- sh -c "test -f /mnt/secrets-store/private_key && echo '✓ Private key exists'"
kubectl exec -n otel-demo $podName -- sh -c "test -f /mnt/secrets-store/ca_chain && echo '✓ CA chain exists'"
```

✅ All three files should exist

### 8.4: View Certificate Content

```powershell
kubectl exec -n otel-demo $podName -- cat /mnt/secrets-store/certificate
```

✅ Should show certificate content

### 8.5: Run Automated Verification

```powershell
powershell -ExecutionPolicy Bypass -File scripts/proof-certificate-sync.ps1
```

✅ All checks should pass

---

## Quick Deployment Script

For convenience, here's a complete deployment script:

```powershell
# Complete Deployment Script
Write-Host "=== OpenBao Certificate Sync Deployment ===" -ForegroundColor Green

Write-Host "`nStep 1: Deploying OpenBao..." -ForegroundColor Yellow
kubectl apply -f kubectl/openbao-deployment.yaml
kubectl wait --for=condition=ready pod -l app=openbao -n openbao --timeout=120s

Write-Host "`nStep 2: Generating certificates..." -ForegroundColor Yellow
powershell -ExecutionPolicy Bypass -File scripts/setup-openbao-certs.ps1

Write-Host "`nStep 3: Installing CSI drivers..." -ForegroundColor Yellow
powershell -ExecutionPolicy Bypass -File scripts/setup-openbao-csi.ps1

Write-Host "`nStep 4: Setting up Kubernetes authentication..." -ForegroundColor Yellow
$podName = kubectl get pods -n openbao -l app=openbao -o jsonpath='{.items[0].metadata.name}'
$logs = kubectl logs -n openbao $podName
$tokenMatch = $logs | Select-String -Pattern "Root Token:\s+(.+)" | ForEach-Object { $_.Matches.Groups[1].Value }
$OPENBAO_TOKEN = $tokenMatch.Trim()

# Enable Kubernetes auth
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao auth enable kubernetes" 2>$null

# Configure Kubernetes auth
$K8S_HOST = "https://kubernetes.default.svc"
$saToken = kubectl exec -n openbao $podName -- cat /var/run/secrets/kubernetes.io/serviceaccount/token
kubectl exec -n openbao $podName -- sh -c "cat /var/run/secrets/kubernetes.io/serviceaccount/ca.crt > /tmp/k8s-ca.crt"
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao write auth/kubernetes/config token_reviewer_jwt='$saToken' kubernetes_host='$K8S_HOST' kubernetes_ca_cert=@/tmp/k8s-ca.crt disable_iss_validation=true"

# Create policy
$policyBase64 = "cGF0aCAiY2VydHMvZGF0YS8qIiB7CiAgY2FwYWJpbGl0aWVzID0gWyJyZWFkIl0KfQ=="
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && echo '$policyBase64' | base64 -d | bao policy write otelcol1-policy -"

# Create role
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao write auth/kubernetes/role/otelcol1-role bound_service_account_names=otelcol1 bound_service_account_namespaces=otel-demo policies=otelcol1-policy ttl=1h"

Write-Host "`nStep 5: Applying SecretProviderClass..." -ForegroundColor Yellow
kubectl apply -f kubectl/openbao-csi-secretproviderclass.yaml

Write-Host "`nStep 6: Deploying collector..." -ForegroundColor Yellow
kubectl apply -f kubectl/otelcol1-with-csi.yaml
kubectl rollout status deployment/otelcol1 -n otel-demo --timeout=120s

Write-Host "`n=== Deployment Complete ===" -ForegroundColor Green
Write-Host "`nVerifying deployment..." -ForegroundColor Yellow
powershell -ExecutionPolicy Bypass -File scripts/proof-certificate-sync.ps1
```

---

## Troubleshooting

### Pod Stuck in Init State

Check pod events:
```powershell
kubectl describe pod -n otel-demo -l app=otelcol1 | Select-String -Pattern "Events:" -Context 10
```

Common issues:
- **CSI driver not installed** → Run `setup-openbao-csi.ps1` again
- **Kubernetes auth not configured** → Complete Step 5
- **Certificates missing** → Run `setup-openbao-certs.ps1` again

### Authentication Failures

Check OpenBao logs:
```powershell
kubectl logs -n openbao -l app=openbao --tail=20 | Select-String -Pattern "403|permission|auth"
```

Verify Kubernetes auth is configured:
```powershell
$podName = kubectl get pods -n openbao -l app=openbao -o jsonpath='{.items[0].metadata.name}'
$logs = kubectl logs -n openbao $podName
$tokenMatch = $logs | Select-String -Pattern "Root Token:\s+(.+)" | ForEach-Object { $_.Matches.Groups[1].Value }
$OPENBAO_TOKEN = $tokenMatch.Trim()
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao read auth/kubernetes/role/otelcol1-role"
```

### Certificates Not Found

Verify certificates exist in OpenBao:
```powershell
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao kv get certs/data/test1"
```

If missing, run certificate setup again:
```powershell
powershell -ExecutionPolicy Bypass -File scripts/setup-openbao-certs.ps1
```

---

## Post-Deployment

After successful deployment:

1. **Certificates are available at:**
   - `/mnt/secrets-store/` in the pod (CSI direct mount from OpenBao)
   - `/etc/otelcol/certs/` (Kubernetes secret mount, if synced)

**Important:** Certificates are stored **ONLY in OpenBao**. The CSI provider fetches them directly from OpenBao when the pod starts - they are NOT stored in Kubernetes first, then copied to the pod. Both mount points get their data directly from OpenBao. See `CERTIFICATE-FLOW-EXPLAINED.md` for detailed flow diagram.

2. **Configure your application** to use certificates from these paths

3. **Monitor certificate rotation** if you set it up

4. **For production:**
   - Set up proper OpenBao seal/unseal
   - Enable TLS for OpenBao
   - Use encrypted storage
   - Implement certificate rotation

---

## Cleanup (if needed)

To remove everything:

```powershell
kubectl delete -f kubectl/otelcol1-with-csi.yaml
kubectl delete -f kubectl/openbao-csi-secretproviderclass.yaml
kubectl delete -f kubectl/openbao-csi-rbac.yaml
kubectl delete secret openbao-token -n otel-demo 2>$null
kubectl delete secret otelcol1-certs -n otel-demo 2>$null
kubectl delete -f kubectl/openbao-deployment.yaml
```

---

## Summary Checklist

- [ ] Cluster access verified
- [ ] OpenBao deployed and running
- [ ] Certificates generated and stored
- [ ] CSI drivers installed
- [ ] Kubernetes auth enabled in OpenBao
- [ ] Kubernetes auth configured
- [ ] Policy created (`otelcol1-policy`)
- [ ] Role created (`otelcol1-role`)
- [ ] SecretProviderClass applied
- [ ] Collector deployed
- [ ] Pod running successfully
- [ ] Certificates mounted and accessible
- [ ] Verification script passes

**Once all items are checked, your deployment is complete!** 🎉
