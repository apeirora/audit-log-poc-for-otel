# OpenBao Files Analysis

## Files Status

### ✅ Moved to openBaoSync/ (All OpenBao-related files)

All OpenBao-specific files have been moved to the `openBaoSync/` folder.

### ❌ Remaining CSI Files in kubectl/ (OLD - Not Used)

The following files in `kubectl/` are **old implementations** that are **NOT referenced** by any scripts:

1. **`kubectl/csi-driver-rbac.yaml`**
   - Generic CSI driver RBAC configuration
   - **Status**: OLD - Not used
   - **Reason**: Scripts now install CSI driver from remote URL

2. **`kubectl/csi-secretproviderclass-crd.yaml`**
   - SecretProviderClass CRD definition
   - **Status**: OLD - Not used
   - **Reason**: CRD is installed automatically from remote manifest

3. **`kubectl/install-csi-driver-complete.yaml`**
   - Complete CSI driver installation manifest
   - **Status**: OLD - Not used
   - **Reason**: Scripts use remote URLs instead

4. **`kubectl/secretproviderclasspodstatus-crd.yaml`**
   - SecretProviderClassPodStatus CRD definition
   - **Status**: OLD - Not used
   - **Reason**: CRD is installed automatically from remote manifest

**Recommendation**: These files can be safely deleted as they are not used by any current scripts.

## Script Path Updates Needed

The following scripts reference files with `kubectl/` paths that need to be updated to `openBaoSync/kubectl/`:

1. **`openBaoSync/scripts/setup-openbao-csi.ps1`**
   - Line 80: `kubectl/openbao-csi-rbac.yaml`
   - Line 83: `kubectl/openbao-csi-secretproviderclass.yaml`
   - Line 86: `kubectl/otelcol1-with-csi.yaml`

2. **`openBaoSync/scripts/setup-openbao-csi.sh`**
   - Line 59: `kubectl/openbao-csi-rbac.yaml`
   - Line 63: `kubectl/openbao-csi-secretproviderclass.yaml`
   - Line 67: `kubectl/otelcol1-with-csi.yaml`

3. **`openBaoSync/scripts/deploy-complete.ps1`**
   - Line 9: `kubectl/openbao-deployment.yaml`
   - Line 24: `scripts/setup-openbao-certs.ps1`
   - Line 32: `scripts/setup-openbao-csi.ps1`
   - Line 85: `kubectl/openbao-csi-secretproviderclass.yaml`
   - Line 93: `kubectl/otelcol1-with-csi.yaml`
   - Line 110: `scripts/proof-certificate-sync.ps1`

## Documentation Path Updates Needed

The following documentation files reference paths that need updating:

1. **`openBaoSync/README-OPENBAO.md`**
   - Multiple references to `kubectl/` and `scripts/` paths

2. **`openBaoSync/DEPLOYMENT-GUIDE.md`**
   - Multiple references to `kubectl/` and `scripts/` paths

## Summary

- **OpenBao-specific files**: ✅ All moved to `openBaoSync/`
- **Old CSI files**: ❌ Can be deleted (not used)
- **Script paths**: ⚠️ Need to be updated to use `openBaoSync/kubectl/` and `openBaoSync/scripts/`
- **Documentation paths**: ⚠️ Need to be updated to reflect new structure
