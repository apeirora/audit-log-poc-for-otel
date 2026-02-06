# OpenBao Certificate Management with CSI Provider

This setup provides automated certificate management using OpenBao (a Vault fork) with the **OpenBao CSI Provider** in your Kubernetes cluster. Certificates are stored in OpenBao's KV store and automatically mounted into pods via CSI volumes, following the [official OpenBao CSI Provider documentation](https://openbao.org/docs/platform/k8s/csi/examples/).

## Overview

The solution uses the **OpenBao CSI Provider** which:
- Automatically mounts secrets as files in pods
- Optionally syncs secrets to Kubernetes secrets
- Uses native Kubernetes CSI volume interface
- No init containers or manual scripts needed
- Supports automatic secret rotation

## Architecture

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

## Prerequisites

- Kubernetes cluster running (kind, minikube, etc.)
- `kubectl` configured and working
- PowerShell (for Windows) or Bash (for Linux/Mac)
- **CSI Secret Store Driver** (installed automatically by setup script)
- **OpenBao CSI Provider** (installed automatically by setup script)

## Setup Order

### Step 1: Deploy OpenBao

Deploy OpenBao to your Kubernetes cluster:

```powershell
kubectl apply -f kubectl/openbao-deployment.yaml
```

Wait for OpenBao to be ready:

```powershell
kubectl wait --for=condition=ready pod -l app=openbao -n openbao --timeout=120s
```

### Step 2: Generate and Store Certificates

Run the setup script to:
- Enable PKI secrets engine
- Generate root CA
- Create test certificates (`test1.test.local` and `test2.test.local`)
- Store certificates in OpenBao KV store

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
4. Create a test role for certificate generation
5. Generate two test certificates
6. Store certificates in OpenBao KV store (as `test1` and `test2`)

### Step 3: Setup CSI Provider Integration

Install CSI drivers and configure the integration:

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

### Step 4: Verify

Check that the secret was created by CSI provider:

```powershell
kubectl get secret otelcol1-certs -n otel-demo
```

View the certificate files in the secret:

```powershell
kubectl get secret otelcol1-certs -n otel-demo -o jsonpath='{.data}' | ConvertFrom-Json | Get-Member -MemberType NoteProperty
```

Check that certificates are mounted in the pod:

```powershell
kubectl exec -n otel-demo <otelcol1-pod-name> -- ls -la /mnt/secrets-store/
```

## How It Works

### CSI Provider Flow

1. **SecretProviderClass** defines:
   - Which secrets to fetch from OpenBao (`certs/data/test1`)
   - How to authenticate (token auth for dev mode)
   - Which keys to extract (`certificate`, `private_key`, `ca_chain`)

2. **CSI Driver** mounts the secrets:
   - Secrets are mounted as files at `/mnt/secrets-store/`
   - Files are named by `objectName` from SecretProviderClass
   - Mount happens automatically when pod starts

3. **Secret Sync** (optional):
   - If `secretObjects` is defined in SecretProviderClass
   - CSI driver creates/updates Kubernetes secret
   - Secret is available for environment variables or other mounts

4. **Pod Access**:
   - Certificates available at `/mnt/secrets-store/certificate`
   - Certificates available at `/mnt/secrets-store/private_key`
   - Certificates available at `/mnt/secrets-store/ca_chain`
   - Kubernetes secret `otelcol1-certs` contains the same data

## File Descriptions

### Deployment Files

#### `kubectl/openbao-deployment.yaml`
- Deploys OpenBao in dev mode
- Creates namespace, service account, deployment, and service
- OpenBao runs unsealed with a root token (dev mode only)
- **Note:** Dev mode should NOT be used in production

#### `kubectl/openbao-csi-rbac.yaml`
- Creates ServiceAccount `otelcol1` in `otel-demo` namespace
- Grants permissions to create/update secrets (for CSI secret sync)
- Required for CSI provider to sync secrets to Kubernetes

#### `kubectl/openbao-csi-secretproviderclass.yaml`
- **SecretProviderClass** resource defining:
  - OpenBao connection details
  - Authentication method (token for dev mode)
  - Which secrets to fetch from KV store
  - How to map to Kubernetes secret

#### `kubectl/otelcol1-with-csi.yaml`
- Updated `otelcol1` deployment with:
  - CSI volume mount for OpenBao certificates
  - ServiceAccount reference
  - No init containers needed!

### Script Files

#### `scripts/setup-openbao-certs.ps1` / `scripts/setup-openbao-certs.sh`
- Sets up OpenBao PKI
- Generates root CA and test certificates
- Stores certificates in OpenBao KV store
- **Run once** after deploying OpenBao

#### `scripts/setup-openbao-csi.ps1` / `scripts/setup-openbao-csi.sh`
- Installs CSI Secret Store Driver
- Installs OpenBao CSI Provider
- Configures token authentication
- Applies all CSI-related resources
- **Run once** after certificates are stored

## Configuration

### Changing Certificate Name

To use a different certificate (e.g., `test2` instead of `test1`), update the SecretProviderClass:

```yaml
objects: |
  - objectName: "certificate"
    secretPath: "certs/data/test2"  # Change from test1 to test2
    secretKey: "certificate"
  # ... other objects
```

Then apply the updated SecretProviderClass:

```powershell
kubectl apply -f kubectl/openbao-csi-secretproviderclass.yaml
```

### Authentication Methods

**Current (Dev Mode):**
- Uses token authentication
- Token stored in Kubernetes secret
- Suitable for development/testing only

**Production (Recommended):**
- Use Kubernetes authentication method
- Pods authenticate using ServiceAccount tokens
- More secure, no token storage needed
- See [OpenBao Kubernetes Auth documentation](https://openbao.org/docs/auth/kubernetes)

## Certificate Locations

### In OpenBao KV Store
- Path: `certs/data/test1` and `certs/data/test2`
- Contains: `certificate`, `private_key`, `ca_chain`

### In Pod (CSI Mount)
- Mount path: `/mnt/secrets-store/`
- Files:
  - `certificate` - Certificate content
  - `private_key` - Private key content
  - `ca_chain` - CA chain content

### In Kubernetes Secret (Synced)
- Name: `otelcol1-certs` (configurable)
- Namespace: `otel-demo` (configurable)
- Keys:
  - `cert.crt` - Certificate
  - `cert.key` - Private key
  - `ca.crt` - CA chain

## Troubleshooting

### CSI Driver Not Installed

Check if CSI Secret Store Driver is running:

```powershell
kubectl get pods -n kube-system -l app=secrets-store-csi-driver
```

If not installed, the setup script will install it automatically.

### CSI Provider Not Installed

Check if OpenBao CSI Provider is running:

```powershell
kubectl get pods -n kube-system -l app=openbao-csi-provider
```

If not installed, the setup script will install it automatically.

### Secret Not Created

Check SecretProviderClass:

```powershell
kubectl describe secretproviderclass openbao-certificates -n otel-demo
```

Check pod events:

```powershell
kubectl describe pod <otelcol1-pod-name> -n otel-demo
```

Check CSI provider logs:

```powershell
kubectl logs -n kube-system -l app=openbao-csi-provider
```

### Authentication Issues

Verify token secret exists:

```powershell
kubectl get secret openbao-token -n otel-demo
```

Check if token is valid:

```powershell
$token = kubectl get secret openbao-token -n otel-demo -o jsonpath='{.data.token}' | ForEach-Object { [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($_)) }
$podName = kubectl get pods -n openbao -l app=openbao -o jsonpath='{.items[0].metadata.name}'
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$token' && bao status"
```

### Certificate Not Found in OpenBao

List certificates in KV store:

```powershell
$podName = kubectl get pods -n openbao -l app=openbao -o jsonpath='{.items[0].metadata.name}'
$token = kubectl logs -n openbao $podName | Select-String "Root Token:" | ForEach-Object { $_.Matches.Groups[1].Value }
kubectl exec -n openbao $podName -- sh -c "export VAULT_ADDR='http://127.0.0.1:8200' && export VAULT_TOKEN='$token' && bao kv list certs/data"
```

## Cleanup

To remove everything:

```powershell
kubectl delete -f kubectl/otelcol1-with-csi.yaml
kubectl delete -f kubectl/openbao-csi-secretproviderclass.yaml
kubectl delete -f kubectl/openbao-csi-rbac.yaml
kubectl delete secret openbao-token -n otel-demo
kubectl delete secret otelcol1-certs -n otel-demo
kubectl delete -f kubectl/openbao-deployment.yaml
```

To remove CSI providers (optional):

```powershell
kubectl delete -f https://raw.githubusercontent.com/openbao/openbao-csi-provider/main/deploy/install.yaml
kubectl delete -f https://raw.githubusercontent.com/kubernetes-sigs/secrets-store-csi-driver/v1.4.0/deploy/secrets-store-csi-driver.yaml
```

## Security Notes

⚠️ **Important:**
- This setup uses OpenBao in **dev mode** which is **NOT suitable for production**
- Root tokens are stored in pod logs (dev mode only)
- Token authentication is used (simpler but less secure)
- Certificates are stored in plain text in OpenBao KV store
- For production, use:
  - Proper OpenBao deployment with seal/unseal
  - Kubernetes authentication method (not token)
  - Encrypted storage
  - Proper RBAC policies
  - TLS for OpenBao communication

## Benefits of CSI Provider Approach

✅ **Native Kubernetes integration** - Uses standard CSI volume interface  
✅ **No init containers** - Secrets mounted directly by CSI driver  
✅ **Automatic rotation** - CSI driver can refresh secrets  
✅ **Better security** - Proper authentication methods supported  
✅ **Simpler configuration** - Declarative SecretProviderClass  
✅ **Production ready** - Follows OpenBao's official recommendations  

## Next Steps

1. Configure your application to use certificates from `/mnt/secrets-store/` or the Kubernetes secret
2. Set up certificate rotation if needed
3. Migrate to Kubernetes authentication for production
4. Consider using OpenBao's PKI for dynamic certificate generation
5. Implement proper secret management for production

## References

- [OpenBao CSI Provider Documentation](https://openbao.org/docs/platform/k8s/csi/examples/)
- [OpenBao Kubernetes Integration](https://openbao.org/docs/platform/k8s/)
- [CSI Secret Store Driver](https://secrets-store-csi-driver.sigs.k8s.io/)
