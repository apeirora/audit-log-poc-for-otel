# OpenBao Certificate Management for OpenTelemetry Collector

This setup provides automated certificate management using OpenBao (a Vault fork) with two approaches:
1. **CSI Provider** - Mounts certificates as files via Kubernetes CSI volumes
2. **Config Source Provider** - Injects secrets directly into collector configuration

## Overview

### CSI Provider Approach (Primary)

The solution uses the **OpenBao CSI Provider** which:
- Automatically mounts secrets as files in pods
- Optionally syncs secrets to Kubernetes secrets
- Uses native Kubernetes CSI volume interface
- No init containers or manual scripts needed
- Supports automatic secret rotation

**Architecture:**
```
OpenBao (KV Store)
    ↓
SecretProviderClass (defines what to fetch)
    ↓
CSI Driver (mounts secrets)
    ↓
Pod Volume Mount (/mnt/secrets-store)
    ↓
Kubernetes Secret (optional sync)
```

### Config Source Provider Approach (Alternative)

Uses OpenTelemetry Collector's built-in **Vault config source provider**:
- Secrets injected directly into config using `${vault:path#key}` syntax
- No CSI drivers required
- Simpler Kubernetes setup
- Only works with OpenTelemetry Collector

**When to use:**
- ✅ Use CSI if you need certificates for multiple applications or prefer file-based access
- ✅ Use Config Source if you only need secrets for Collector and want simpler setup

## Prerequisites

- Kubernetes cluster running (kind, minikube, etc.)
- `kubectl` configured and working
- PowerShell (for Windows) or Bash (for Linux/Mac)
- **CSI Secret Store Driver** (installed automatically by setup script)
- **OpenBao CSI Provider** (installed automatically by setup script)

## Quick Start

### Step 1: Deploy OpenBao

```powershell
kubectl apply -f kubectl/openbao-deployment.yaml
kubectl wait --for=condition=ready pod -l app=openbao -n openbao --timeout=120s
```

### Step 2: Generate and Store Certificates

**PowerShell (Windows):**
```powershell
powershell -ExecutionPolicy Bypass -File scripts/setup-openbao-certs.ps1
```

**Bash (Linux/Mac):**
```bash
bash scripts/setup-openbao-certs.sh
```

This script will:
1. Extract the root token from OpenBao logs
2. Enable PKI secrets engine
3. Generate a root CA
4. Create test certificates (`test1.test.local` and `test2.test.local`)
5. Store certificates in OpenBao KV store

### Step 3: Setup CSI Provider Integration

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
3. Extract OpenBao root token and create a secret
4. Apply RBAC configuration
5. Create SecretProviderClass
6. Update `otelcol1` deployment with CSI volumes

### Step 4: Setup Kubernetes Authentication (Required)

**Important:** The setup script configures token auth, but we use Kubernetes authentication for better security.

1. **Enable Kubernetes Auth in OpenBao:**
   ```powershell
   $podName = kubectl get pods -n openbao -l app=openbao -o jsonpath='{.items[0].metadata.name}'
   $logs = kubectl logs -n openbao $podName
   $tokenMatch = $logs | Select-String -Pattern "Root Token:\s+(.+)" | ForEach-Object { $_.Matches.Groups[1].Value }
   $OPENBAO_TOKEN = $tokenMatch.Trim()
   kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao auth enable kubernetes"
   ```

2. **Configure Kubernetes Auth:**
   ```powershell
   $K8S_HOST = "https://kubernetes.default.svc"
   $saToken = kubectl exec -n openbao $podName -- cat /var/run/secrets/kubernetes.io/serviceaccount/token
   $caCert = kubectl exec -n openbao $podName -- cat /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
   kubectl exec -n openbao $podName -- sh -c "cat /var/run/secrets/kubernetes.io/serviceaccount/ca.crt > /tmp/k8s-ca.crt"
   kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao write auth/kubernetes/config token_reviewer_jwt='$saToken' kubernetes_host='$K8S_HOST' kubernetes_ca_cert=@/tmp/k8s-ca.crt disable_iss_validation=true"
   ```

3. **Create Policy:**
   ```powershell
   $policyBase64 = "cGF0aCAiY2VydHMvZGF0YS8qIiB7CiAgY2FwYWJpbGl0aWVzID0gWyJyZWFkIl0KfQ=="
   kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && echo '$policyBase64' | base64 -d | bao policy write otelcol1-policy -"
   ```

4. **Create Role:**
   ```powershell
   kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao write auth/kubernetes/role/otelcol1-role bound_service_account_names=otelcol1 bound_service_account_namespaces=otel-demo policies=otelcol1-policy ttl=1h"
   ```

5. **Update SecretProviderClass:**
   ```powershell
   kubectl apply -f kubectl/openbao-csi-secretproviderclass.yaml
   ```

### Step 5: Verify

Check that certificates are mounted in the pod:

```powershell
kubectl get secret otelcol1-certs -n otel-demo
$podName = kubectl get pods -n otel-demo -l app=otelcol1 -o jsonpath='{.items[0].metadata.name}'
kubectl exec -n otel-demo $podName -- ls -la /mnt/secrets-store/
```

## How It Works

### Certificate Flow

Certificates are stored **ONLY in OpenBao KV Store**. When a pod starts:

1. **CSI Provider** authenticates to OpenBao using Kubernetes auth (ServiceAccount token)
2. Fetches certificates from `certs/data/test1` in OpenBao
3. Mounts them directly as files at `/mnt/secrets-store/`
4. Optionally creates/updates Kubernetes secret `otelcol1-certs`

**Key Point:** Certificates are fetched directly from OpenBao on pod start - they are NOT stored in Kubernetes first.

### Certificate Locations

**In OpenBao KV Store:**
- Path: `certs/data/test1` and `certs/data/test2`
- Contains: `certificate`, `private_key`, `ca_chain`

**In Pod (CSI Mount):**
- Mount path: `/mnt/secrets-store/`
- Files: `certificate`, `private_key`, `ca_chain`

**In Kubernetes Secret (Synced):**
- Name: `otelcol1-certs`
- Namespace: `otel-demo`
- Keys: `cert.crt`, `cert.key`, `ca.crt`

## Configuration

### Changing Certificate Name

To use a different certificate (e.g., `test2` instead of `test1`), update the SecretProviderClass:

```yaml
objects: |
  - objectName: "certificate"
    secretPath: "certs/data/test2"
    secretKey: "certificate"
```

Then apply:
```powershell
kubectl apply -f kubectl/openbao-csi-secretproviderclass.yaml
```

### Authentication Methods

**Current Implementation (Kubernetes Auth):**
- ✅ Uses Kubernetes authentication method
- ✅ Pods authenticate using ServiceAccount tokens
- ✅ No token storage needed (more secure)
- ✅ Production-ready approach

**Alternative (Token Auth - Not Used):**
- Uses token authentication
- Token stored in Kubernetes secret `openbao-token`
- Simpler setup but less secure
- Suitable for development/testing only

## Alternative: Config Source Provider

For a simpler setup without CSI drivers, use the Vault Config Source Provider:

### Configuration

Add to collector config:

```yaml
config_sources:
  vault:
    endpoint: "http://openbao.openbao.svc.cluster.local:8200"
    auth:
      method: kubernetes
      mount_path: "auth/kubernetes"
      role: "otelcol1-role"
    poll_interval: 5m

exporters:
  otlp:
    tls:
      cert_pem: "${vault:certs/data/test1#certificate}"
      key_pem: "${vault:certs/data/test1#private_key}"
```

See `vault-config-source/` folder for complete example.

## File Structure

```
openBaoSync/
├── kubectl/
│   ├── openbao-deployment.yaml          # OpenBao deployment
│   ├── openbao-csi-rbac.yaml             # RBAC for CSI
│   ├── openbao-csi-secretproviderclass.yaml  # SecretProviderClass
│   └── otelcol1-with-csi.yaml            # Collector with CSI volumes
├── scripts/
│   ├── setup-openbao-certs.ps1/.sh       # Certificate generation
│   ├── setup-openbao-csi.ps1/.sh         # CSI setup
│   └── proof-certificate-sync.ps1        # Verification script
└── vault-config-source/
    └── kubectl/
        └── otelcol1-with-vault-config-source.yaml  # Config source example
```

## Troubleshooting

### CSI Driver Not Installed

```powershell
kubectl get pods -n kube-system -l app=secrets-store-csi-driver
```

If not installed, run `setup-openbao-csi.ps1` again.

### Authentication Failures

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
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$OPENBAO_TOKEN' && bao kv get certs/test1"
```

## Cleanup

To remove everything:

```powershell
kubectl delete -f kubectl/otelcol1-with-csi.yaml
kubectl delete -f kubectl/openbao-csi-secretproviderclass.yaml
kubectl delete -f kubectl/openbao-csi-rbac.yaml
kubectl delete secret openbao-token -n otel-demo 2>$null
kubectl delete secret otelcol1-certs -n otel-demo 2>$null
kubectl delete -f kubectl/openbao-deployment.yaml
```

## Security Notes

⚠️ **Important:**
- This setup uses OpenBao in **dev mode** which is **NOT suitable for production**
- Root tokens are stored in pod logs (dev mode only)
- **Kubernetes authentication is used** (production-ready approach)
- Certificates are stored in plain text in OpenBao KV store
- For production, additionally use:
  - Proper OpenBao deployment with seal/unseal
  - Encrypted storage
  - TLS for OpenBao communication
  - Proper certificate rotation

## Benefits

✅ **Native Kubernetes integration** - Uses standard CSI volume interface  
✅ **No init containers** - Secrets mounted directly by CSI driver  
✅ **Automatic rotation** - CSI driver can refresh secrets  
✅ **Better security** - Proper authentication methods supported  
✅ **Simpler configuration** - Declarative SecretProviderClass  
✅ **Production ready** - Follows OpenBao's official recommendations  

## References

- [OpenBao CSI Provider Documentation](https://openbao.org/docs/platform/k8s/csi/examples/)
- [OpenBao Kubernetes Integration](https://openbao.org/docs/platform/k8s/)
- [CSI Secret Store Driver](https://secrets-store-csi-driver.sigs.k8s.io/)
- [OpenTelemetry Collector Config Sources](https://opentelemetry.io/docs/collector/configuration/#config-sources)
