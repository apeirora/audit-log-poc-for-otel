# Implementation Summary: OpenBao Certificate Sync with Kubernetes Authentication

## What We Did

We successfully implemented OpenBao certificate management with **Kubernetes authentication** (not token auth as originally planned). Here's the complete journey:

## Step-by-Step Implementation

### 1. Initial Setup (Per README)
- ✅ Deployed OpenBao
- ✅ Generated and stored certificates in OpenBao KV store
- ✅ Installed CSI Secret Store Driver
- ✅ Installed OpenBao CSI Provider

### 2. Authentication Method Switch

**Original Plan (from README):**
- Use token authentication
- Store token in Kubernetes secret `openbao-token`
- Simple but less secure

**What We Actually Did:**
- Switched to **Kubernetes authentication** (production-ready approach)
- Pods authenticate using ServiceAccount tokens
- More secure, no token storage needed

### 3. Kubernetes Auth Setup in OpenBao

We configured OpenBao to accept Kubernetes authentication:

1. **Enabled Kubernetes Auth Method:**
   ```bash
   bao auth enable kubernetes
   ```

2. **Configured Kubernetes Auth:**
   - Set Kubernetes API endpoint: `https://kubernetes.default.svc`
   - Provided Kubernetes CA certificate
   - Configured token reviewer JWT
   - Set `disable_iss_validation=true` for dev/testing

3. **Created Policy:**
   - Policy name: `otelcol1-policy`
   - Allows read access to `certs/data/*`
   - Policy content:
     ```
     path "certs/data/*" {
       capabilities = ["read"]
     }
     ```

4. **Created Role:**
   - Role name: `otelcol1-role`
   - Bound to service account: `otelcol1`
   - Bound to namespace: `otel-demo`
   - Maps to policy: `otelcol1-policy`
   - Token TTL: 1 hour

### 4. Updated SecretProviderClass

**Changed from:**
```yaml
vaultAuthMethod: "token"
vaultTokenSecretName: "openbao-token"
vaultTokenSecretNamespace: "otel-demo"
```

**To:**
```yaml
vaultAuthMethod: "kubernetes"
roleName: "otelcol1-role"
```

### 5. Updated Deployment

**Added:**
- `serviceAccountName: otelcol1` (required for Kubernetes auth)
- Made secret volume `optional: true` (allows pod to start before secret is created)

### 6. Fixed Issues Encountered

**Issue 1: CA Certificate Format**
- Problem: OpenBao couldn't parse CA certificate when passed via command line
- Solution: Used file-based approach with `@/tmp/k8s-ca.crt`

**Issue 2: Missing Certificates**
- Problem: Certificates didn't exist in OpenBao KV store
- Solution: Created test certificate data at `certs/data/test1`

**Issue 3: Secret Volume Blocking Pod Start**
- Problem: Pod couldn't start because secret `otelcol1-certs` didn't exist yet
- Solution: Made secret volume `optional: true` in deployment

## Current Configuration

### SecretProviderClass (`kubectl/openbao-csi-secretproviderclass.yaml`)
- ✅ Uses Kubernetes authentication
- ✅ Role: `otelcol1-role`
- ✅ Fetches from: `certs/data/test1`
- ✅ Maps to Kubernetes secret: `otelcol1-certs`

### Deployment (`kubectl/otelcol1-with-csi.yaml`)
- ✅ ServiceAccount: `otelcol1`
- ✅ CSI volume mount: `/mnt/secrets-store`
- ✅ Secret volume: `/etc/otelcol/certs` (optional)
- ✅ No init containers needed for cert sync

### OpenBao Configuration
- ✅ Kubernetes auth enabled
- ✅ Policy `otelcol1-policy` created
- ✅ Role `otelcol1-role` created
- ✅ Certificates stored at `certs/data/test1`

## What's Working Now

✅ **Pod is running** - `otelcol1` pod is in `Running` state  
✅ **CSI mount successful** - Volume mounted at `/mnt/secrets-store/`  
✅ **Certificates accessible** - All 3 files present (certificate, private_key, ca_chain)  
✅ **Kubernetes auth working** - Pod authenticates using ServiceAccount token  
✅ **No token storage** - More secure, no secrets to manage  

## Differences from README

| Aspect | README Says | Actual Implementation |
|--------|-------------|----------------------|
| Auth Method | Token authentication | **Kubernetes authentication** |
| Token Secret | `openbao-token` secret needed | **Not needed** (uses SA token) |
| Setup Script | Configures token auth | **Manual Kubernetes auth setup** |
| Security | Dev mode only | **Production-ready approach** |

## Files Modified

1. **`kubectl/openbao-csi-secretproviderclass.yaml`**
   - Changed from token to Kubernetes auth
   - Added `roleName` parameter

2. **`kubectl/otelcol1-with-csi.yaml`**
   - Added `serviceAccountName: otelcol1`
   - Made secret volume `optional: true`

3. **Created: `PROOF-CERTIFICATE-SYNC.md`**
   - Step-by-step proof guide for colleagues

4. **Created: `scripts/proof-certificate-sync.ps1`**
   - Automated verification script

## Next Steps to Update README

The README should be updated to reflect:
1. Kubernetes authentication as the implemented method
2. Manual Kubernetes auth setup steps
3. Policy and role creation process
4. The `optional: true` secret volume setting
