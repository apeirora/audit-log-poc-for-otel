# Tampering Resistance - Quick Action Plan

## Summary of Tampering Points

### Critical Tampering Points Identified

1. **Network Transmission** (Application → Sidecar → Collector → Sink)
   - Man-in-the-middle attacks
   - Packet injection/modification
   - Replay attacks

2. **Collector Internal Processing**
   - Receiver compromise/modification
   - Processor tampering (unauthorized modifications)
   - Exporter data leakage

3. **Configuration Tampering**
   - Unauthorized config changes
   - Runtime modification

## Immediate Actions (Use Existing Features)

### 1. Enable TLS/mTLS Everywhere ✅

**Application → Sidecar Collector:**
```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
        tls:
          cert_file: /etc/otel/server.crt
          key_file: /etc/otel/server.key
          client_ca_file: /etc/otel/ca.crt
          client_auth_type: RequireAndVerifyClientCert
```

**Collector → Sink:**
```yaml
exporters:
  otlp:
    endpoint: sink.example.com:4317
    tls:
      ca_file: /etc/otel/ca.crt
      cert_file: /etc/otel/client.crt
      key_file: /etc/otel/client.key
```

### 2. Add Authentication ✅

```yaml
extensions:
  oauth2client:
    client_id: ${OAUTH_CLIENT_ID}
    client_secret: ${OAUTH_CLIENT_SECRET}
    token_url: https://auth.example.com/token

receivers:
  otlp:
    protocols:
      grpc:
        auth:
          authenticator: oauth2client
```

### 3. Use Schema Processor for Validation ✅

```yaml
processors:
  schema:
    targets:
      - https://opentelemetry.io/schemas/1.26.0
    prefetch:
      - https://opentelemetry.io/schemas/1.26.0
```

This validates data structure and can catch some tampering.

### 4. Use Isolation Forest processor 

  adds inline, unsupervised anomaly detection to any OpenTelemetry Collector pipeline (traces, metrics, or logs). It embeds a lightweight implementation of the Isolation Forest algorithm that automatically learns normal behaviour from recent telemetry and tags, scores, or optionally drops anomalies *in‑flight* – no external ML service required.

  If we implement it, we can say "we use Ai to detect log tampering" +10 point for gryfindor

## New Features Needed

### Priority 1: Data Integrity Processor

**Purpose:** Add HMAC signatures to detect tampering

**Reference Implementation:** `receiver/mongodbatlasreceiver/alerts.go:429-445`

**Proposed Location:** `processor/integrityprocessor/`

**Features:**
- Sign data at source (application/sidecar)
- Verify signatures at collector
- Re-sign for downstream verification
- Support HMAC-SHA256, HMAC-SHA512

**Key Management Options:**
1. **Local Secret Storage** (Simple): Store HMAC secrets in environment variables or config
2. **OpenBao Transit** (Recommended): Use OpenBao Transit secrets engine for centralized key management
   - Centralized key management and rotation
   - No secrets stored in collector config
   - Audit logging of cryptographic operations
   - API-based HMAC generation/verification via HTTP
   - Reference: https://openbao.org/docs/secrets/transit/

### Priority 2: Enhanced OTLP Receiver with HMAC

**Purpose:** Add HMAC verification to standard OTLP receivers

**Reference Implementation:** `receiver/mongodbatlasreceiver/alerts.go:284-303`

**Proposed Enhancement:** Add to `receiver/otlpreceiver/`

**Features:**
- HMAC signature in headers (`X-OTel-HMAC-Signature`)
- Shared secret configuration (local or OpenBao Transit)
- Signature verification before processing
- Reject unsigned/invalid requests

**Key Management Options:**
1. **Local Secret**: Store HMAC secret in environment variable
2. **OpenBao Transit**: Use OpenBao Transit API for HMAC verification
   - No secret storage in collector
   - Automatic key rotation support
   - Centralized key management

### Priority 3: Audit Logging Processor

**Purpose:** Log all data modifications for forensic analysis

**Proposed Location:** `processor/auditprocessor/`

**Features:**
- Log before/after state of modifications
- Track which processor made changes
- Hash-based change detection
- Configurable log levels

### Priority 4: Replay Protection Extension

**Purpose:** Prevent replay attacks

**Proposed Location:** `extension/replayprotectionextension/`

**Features:**
- Nonce generation and validation
- Timestamp-based replay window (e.g., 5 minutes)
- Request ID tracking
- Configurable window size

## Complete Secure Configuration Example

```yaml
extensions:
  oauth2client:
    client_id: ${OAUTH_CLIENT_ID}
    client_secret: ${OAUTH_CLIENT_SECRET}
    token_url: https://auth.example.com/token
  
  replayprotection:
    window: 5m
    nonce_required: true

receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
        tls:
          cert_file: /etc/otel/server.crt
          key_file: /etc/otel/server.key
          client_ca_file: /etc/otel/ca.crt
          client_auth_type: RequireAndVerifyClientCert
        auth:
          authenticator: oauth2client
        # TODO: When implemented
        # hmac:
        #   # Option 1: Local secret
        #   secret: ${HMAC_SECRET}
        #   algorithm: SHA256
        #   # Option 2: OpenBao Transit (recommended)
        #   # openbao_transit:
        #   #   address: https://openbao.example.com:8200
        #   #   token: ${OPENBAO_TOKEN}
        #   #   key_name: otel-hmac-key
        #   #   mount_path: transit

processors:
  batch:
  
  schema:
    targets:
      - https://opentelemetry.io/schemas/1.26.0
  
  # TODO: When implemented
  # integrity:
  #   sign:
  #     algorithm: HMAC-SHA256
  #     # Option 1: Local secret
  #     secret: ${INTEGRITY_SECRET}
  #     # Option 2: OpenBao Transit (recommended)
  #     # openbao_transit:
  #     #   address: https://openbao.example.com:8200
  #     #   token: ${OPENBAO_TOKEN}
  #     #   key_name: otel-hmac-key
  #     #   mount_path: transit
  #   verify: true
  
  # TODO: When implemented
  # audit:
  #   log_level: info
  #   log_changes: true
  #   output: file:///var/log/otel/audit.log

exporters:
  otlp:
    endpoint: sink.example.com:4317
    tls:
      ca_file: /etc/otel/ca.crt
      cert_file: /etc/otel/client.crt
      key_file: /etc/otel/client.key
    auth:
      authenticator: oauth2client

service:
  extensions: [oauth2client, replayprotection]
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch, schema]  # Add integrity, audit when implemented
      exporters: [otlp]
    metrics:
      receivers: [otlp]
      processors: [batch, schema]
      exporters: [otlp]
    logs:
      receivers: [otlp]
      processors: [batch, schema]
      exporters: [otlp]
```

## Implementation Roadmap

### Phase 1: Immediate 
- ✅ Enable TLS/mTLS
- ✅ Add authentication
- ✅ Use schema processor

### Phase 2: Short-term 
- Implement Data Integrity Processor
- Enhance OTLP Receiver with HMAC

### Phase 3: Medium-term 
- Implement Audit Logging Processor
- Implement Replay Protection Extension

## Testing Tampering Resistance

### Test Cases

1. **MITM Attack Test**
   - Intercept traffic without valid cert
   - Should fail TLS handshake

2. **Replay Attack Test**
   - Replay same request multiple times
   - Should be rejected (when replay protection implemented)

3. **Data Modification Test**
   - Modify data in transit
   - Should be detected by HMAC verification

4. **Unauthorized Access Test**
   - Access without valid credentials
   - Should be rejected

5. **Processor Tampering Test**
   - Unauthorized processor modifications
   - Should be logged (when audit processor implemented)

## References

- **Secure Tracing Example:** `examples/secure-tracing/README.md`
- **HMAC Implementation:** `receiver/mongodbatlasreceiver/alerts.go:429-445`
- **Schema Processor:** `processor/schemaprocessor/README.md`
- **Authentication Extensions:** `extension/` directory
- **OpenBao Transit:** https://openbao.org/docs/secrets/transit/ (for centralized HMAC key management)