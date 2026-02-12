# Certificate Flow Explained

## Where Certificates Are Stored

Certificates are stored in **ONE place**: **OpenBao KV Store**

```
OpenBao KV Store (Source of Truth)
└── certs/data/test1
    ├── certificate
    ├── private_key
    └── ca_chain
```

**Important:** Certificates are NOT stored in Kubernetes secrets first. They're fetched directly from OpenBao when needed.

---

## How Certificates Get to the Pod

When your pod starts, the **CSI Provider** fetches certificates from OpenBao and makes them available in **TWO ways**:

### Path 1: Direct CSI Mount (Always Happens)

```
OpenBao KV Store
    ↓
CSI Provider (fetches on pod start)
    ↓
Directly mounted to pod at /mnt/secrets-store/
    ↓
Files available immediately:
  - /mnt/secrets-store/certificate
  - /mnt/secrets-store/private_key
  - /mnt/secrets-store/ca_chain
```

**This is the PRIMARY way** certificates are accessed. The CSI provider:

1. Authenticates to OpenBao using Kubernetes auth
2. Fetches certificates from `certs/data/test1`
3. Mounts them directly as files in the pod
4. **No Kubernetes secret involved** - direct mount from OpenBao

### Path 2: Kubernetes Secret Sync (Optional, Secondary)

```
OpenBao KV Store
    ↓
CSI Provider (fetches on pod start)
    ↓
Creates/updates Kubernetes Secret "otelcol1-certs"
    ↓
Secret mounted to pod at /etc/otelcol/certs/
    ↓
Files available:
  - /etc/otelcol/certs/cert.crt
  - /etc/otelcol/certs/cert.key
  - /etc/otelcol/certs/ca.crt
```

**This is OPTIONAL** and happens because `secretObjects` is defined in SecretProviderClass. The CSI provider:

1. Fetches from OpenBao (same as Path 1)
2. **Additionally** creates/updates a Kubernetes secret
3. The secret is mounted as a regular Kubernetes volume

---

## The Flow When Pod Starts

```
1. Pod starts
   ↓
2. CSI Provider intercepts volume mount request
   ↓
3. CSI Provider authenticates to OpenBao:
   - Uses ServiceAccount token (otelcol1)
   - Authenticates via Kubernetes auth method
   - Gets role "otelcol1-role" → policy "otelcol1-policy"
   ↓
4. CSI Provider fetches from OpenBao:
   - GET certs/data/test1
   - Extracts: certificate, private_key, ca_chain
   ↓
5. CSI Provider mounts files directly:
   - Creates files at /mnt/secrets-store/certificate
   - Creates files at /mnt/secrets-store/private_key
   - Creates files at /mnt/secrets-store/ca_chain
   ↓
6. (Optional) CSI Provider syncs to Kubernetes Secret:
   - Creates/updates secret "otelcol1-certs"
   - Maps: certificate → cert.crt, private_key → cert.key, ca_chain → ca.crt
   ↓
7. Pod container starts
   - Can access certificates from /mnt/secrets-store/ (CSI mount)
   - Can access certificates from /etc/otelcol/certs/ (K8s secret mount, if synced)
```

---

## Key Points

### ❌ What Does NOT Happen

- Certificates are **NOT** stored in Kubernetes first
- Certificates are **NOT** copied from Kubernetes to pod
- The flow is **NOT**: OpenBao → K8s Secret → Pod

### ✅ What Actually Happens

- Certificates are stored **ONLY** in OpenBao
- CSI Provider fetches **directly** from OpenBao when pod starts
- Certificates are mounted **directly** to pod filesystem
- Kubernetes secret is created **in parallel** (optional), not as a step in the chain

---

## Why Two Mount Points?

### `/mnt/secrets-store/` (CSI Direct Mount)

- **Always available** - Direct from OpenBao
- **Real-time** - Fetched fresh on each pod start
- **File names**: `certificate`, `private_key`, `ca_chain`
- **Use this for**: Direct file access, applications that read files

### `/etc/otelcol/certs/` (Kubernetes Secret)

- **Optional** - Only if secret sync is enabled
- **May be delayed** - Secret is created after CSI mount
- **File names**: `cert.crt`, `cert.key`, `ca.crt` (mapped names)
- **Use this for**: Environment variables, other Kubernetes integrations

---

## Visual Flow Diagram

```
┌─────────────────┐
│   OpenBao KV    │  ← Source of Truth (ONLY storage location)
│ certs/data/test1│
└────────┬────────┘
         │
         │ (CSI Provider fetches on pod start)
         ↓
    ┌────────────┐
    │ CSI Provider│
    └─────┬──────┘
          │
          ├─────────────────┬──────────────────┐
          │                 │                  │
          ↓                 ↓                  ↓
    ┌──────────┐    ┌──────────────┐    ┌──────────────┐
    │ Direct    │    │ K8s Secret   │    │ Pod Memory   │
    │ CSI Mount │    │ (optional)   │    │ (ephemeral)  │
    └──────────┘    └──────────────┘    └──────────────┘
          │                 │                  │
          ↓                 ↓                  ↓
    /mnt/secrets-    otelcol1-certs    Files loaded
    store/           secret object      into memory
    (files)          (K8s resource)     when accessed
```

---

## Storage Locations Summary

| Location                | Type         | When Created                 | Persistence              | Purpose                |
| ----------------------- | ------------ | ---------------------------- | ------------------------ | ---------------------- |
| **OpenBao KV**          | Source       | Manual (setup script)        | Permanent                | Single source of truth |
| `/mnt/secrets-store/`   | CSI Mount    | Pod start                    | Ephemeral (pod lifetime) | Direct file access     |
| `otelcol1-certs` Secret | K8s Secret   | Pod start (optional)         | Until deleted            | K8s integration        |
| `/etc/otelcol/certs/`   | Secret Mount | Pod start (if secret exists) | Ephemeral (pod lifetime) | Alternative access     |

---

## Important Notes

1. **OpenBao is the ONLY permanent storage** - Everything else is ephemeral
2. **CSI Provider fetches on-demand** - Not a copy operation, it's a fetch
3. **Two mount points, same source** - Both get data from OpenBao, not from each other
4. **No copying between locations** - Each path fetches independently from OpenBao
5. **Secret sync is optional** - The direct CSI mount always works

---

## Example: What Happens When You Restart a Pod

```
1. Old pod deleted
   ↓
2. New pod starts
   ↓
3. CSI Provider runs (before container starts)
   ↓
4. Authenticates to OpenBao (fresh authentication)
   ↓
5. Fetches certificates from OpenBao (fresh fetch)
   ↓
6. Mounts to /mnt/secrets-store/ (fresh mount)
   ↓
7. (Optional) Updates K8s secret (fresh sync)
   ↓
8. Container starts with certificates available
```

**Key Point:** Every pod restart = fresh fetch from OpenBao. No stale data, no copying from Kubernetes.

---

## Why This Architecture?

✅ **Security**: Certificates never stored in Kubernetes etcd (only in OpenBao)  
✅ **Fresh Data**: Always fetches latest from OpenBao on pod start  
✅ **Flexibility**: Can rotate certificates in OpenBao without touching K8s  
✅ **Performance**: Direct mount, no intermediate storage  
✅ **Audit**: All access goes through OpenBao (can be logged)
