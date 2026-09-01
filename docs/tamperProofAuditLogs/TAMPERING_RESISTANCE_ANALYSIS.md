# Tampering Resistance Analysis for OpenTelemetry Collector Pipeline

## Architecture Overview

```text
Application → Sidecar Collector → OTEL Collector → Sink
```

## Tampering Points Analysis

### 1. Network Transmission Points

#### 1.1 Application → Sidecar Collector

**Tampering Risks:**

- **Malicious:** Man-in-the-middle (MITM) attacks, packet injection, replay attacks
- **Accidental:** Network misconfiguration, proxy interference, packet corruption

**Existing Solutions:**

- ✅ **TLS/mTLS** - Supported by OTLP receivers (gRPC/HTTP)
  - Location: `examples/secure-tracing/README.md`
  - Configuration: TLS with client certificate authentication
- ✅ **Authentication Extensions:**
  - `basicauthextension` - Basic auth
  - `bearertokenauthextension` - Bearer token auth
  - `oidcauthextension` - OIDC authentication
  - `oauth2clientauthextension` - OAuth2 client credentials

**Gaps:**

- ❌ No built-in data integrity verification (HMAC/signatures) for OTLP
- ❌ No replay attack protection (nonce/timestamp validation)
- ❌ Limited authorization granularity (only authentication, not fine-grained authorization)

#### 1.2 Sidecar Collector → OTEL Collector

**Tampering Risks:**

- Same as 1.1, plus:
- Sidecar compromise could inject/modify data
- Configuration tampering in sidecar

**Existing Solutions:**

- Same as 1.1
- ✅ **OTLP Arrow Receiver** has authentication support (`receiver/otelarrowreceiver/internal/arrow/arrow.go:575-582`)

**Gaps:**

- Same as 1.1

#### 1.3 OTEL Collector → Sink

**Tampering Risks:**

- MITM attacks on outbound connections
- Sink compromise could request data replay
- Unauthorized data access at sink

**Existing Solutions:**

- ✅ **TLS support in exporters** - Most exporters support TLS
- ✅ **Authentication in exporters:**
  - Bearer tokens (e.g., `tinybirdexporter`, `sumologicexporter`)
  - API keys (e.g., `coralogixexporter`)
  - AWS SigV4 (`sigv4authextension`)

**Gaps:**

- ❌ No end-to-end data integrity verification
- ❌ No proof of origin for data

### 2. Collector Internal Processing

#### 2.1 Receiver Processing

**Tampering Risks:**

- **Malicious:** Compromised receiver modifying data before processing
- **Accidental:** Receiver bugs causing data corruption
- **Malicious:** Unauthorized data injection via receiver endpoints

**Existing Solutions:**

- ✅ **HMAC Signature Verification** - Implemented in `mongodbatlasreceiver` (`receiver/mongodbatlasreceiver/alerts.go:299-303`)
  - Uses HMAC-SHA1 for payload verification
  - Rejects unsigned or invalidly signed payloads
- ✅ **Content-Length validation** - Prevents oversized payloads
- ✅ **Authentication** - Various auth mechanisms available

**Gaps:**

- ❌ HMAC verification not available for standard OTLP receivers
- ❌ No receiver-level data validation/whitelisting
- ❌ No rate limiting at receiver level (except some specific receivers like `yanggrpcreceiver`)

#### 2.2 Processor Pipeline

**Tampering Risks:**

- **Malicious:** Compromised processor modifying/dropping data
- **Accidental:** Misconfigured processors (e.g., filterprocessor dropping legitimate data)
- **Malicious:** Unauthorized data transformation

**Existing Solutions:**

- ✅ **Processors are explicit** - Configuration-driven, visible in config
- ✅ **Filter processor** - Can filter data based on rules
- ⚠️ **Transform processor** - Powerful but can modify any data

**Gaps:**

- ❌ No audit logging of processor modifications
- ❌ No integrity checks between processors
- ❌ No processor-level authorization/whitelisting
- ❌ No detection of unexpected data modifications
- ❌ No processor execution order validation

#### 2.3 Exporter Processing

**Tampering Risks:**

- **Malicious:** Compromised exporter modifying data before sending
- **Accidental:** Exporter bugs causing data corruption
- **Malicious:** Data exfiltration to unauthorized sinks

**Existing Solutions:**

- ✅ **Exporter authentication** - Various auth mechanisms
- ✅ **TLS** - Encrypted transport

**Gaps:**

- ❌ No exporter-level data integrity verification
- ❌ No audit trail of what was exported
- ❌ No exporter authorization checks

### 3. Configuration and Runtime

#### 3.1 Configuration Tampering

**Tampering Risks:**

- **Malicious:** Unauthorized config changes
- **Accidental:** Misconfiguration leading to data loss/modification

**Existing Solutions:**

- ✅ **Config validation** - Collector validates config on startup
- ✅ **OPAMP extension** - Remote config management with validation

**Gaps:**

- ❌ No config file integrity verification (signatures)
- ❌ No config change audit logging
- ❌ No runtime config modification prevention

#### 3.2 Runtime Environment

**Tampering Risks:**

- **Malicious:** Container/process compromise
- **Malicious:** Memory tampering
- **Accidental:** Resource exhaustion causing data loss

**Existing Solutions:**

- ✅ **Health check extensions** - Monitor collector health
- ✅ **Observability** - Collector emits its own telemetry

**Gaps:**

- ❌ No runtime integrity monitoring
- ❌ No detection of unexpected process behaviore
- ❌ No secure boot/startup verification

### 4. Data Storage (if applicable)

#### 4.1 Queues/Buffers

**Tampering Risks:**

- **Malicious:** Queue manipulation
- **Accidental:** Queue corruption

**Existing Solutions:**

- ✅ **Persistent queues** - Some exporters support persistent storage

**Gaps:**

- ❌ No queue data integrity verification
- ❌ No encrypted queue storage

## Recommended Solutions

### Existing Solutions to Implement

#### 1. Enable TLS/mTLS Everywhere

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
        tls:
          cert_file: /path/to/server.crt
          key_file: /path/to/server.key
          client_ca_file: /path/to/ca.crt
          client_auth_type: RequireAndVerifyClientCert

exporters:
  otlp:
    endpoint: sink.example.com:4317
    tls:
      ca_file: /path/to/ca.crt
      cert_file: /path/to/client.crt
      key_file: /path/to/client.key
```

#### 2. Use Authentication Extensions

```yaml
extensions:
  oauth2client:
    client_id: your-client-id
    client_secret: your-client-secret
    token_url: https://auth.example.com/token

receivers:
  otlp:
    protocols:
      grpc:
        auth:
          authenticator: oauth2client
```

#### 3. Implement HMAC Verification (for webhook-style receivers)

Reference implementation: `receiver/mongodbatlasreceiver/alerts.go:429-445`

### New Features to Implement

#### 1. Data Integrity Processor (NEW)

**Purpose:** Add HMAC signatures to telemetry data for end-to-end integrity verification

**Features:**

- Sign data at source (application/sidecar)
- Verify signatures at collector
- Re-sign for downstream (collector → sink)
- Support multiple algorithms (HMAC-SHA256, HMAC-SHA512)

**Key Management Options:**

- **Local Secret Storage**: Store HMAC secrets in environment variables or config files
- **OpenBao Transit** (Recommended): Use OpenBao Transit secrets engine for centralized key management
  - Centralized key management and rotation
  - No secrets stored in collector config
  - Audit logging of cryptographic operations
  - API-based HMAC generation/verification via HTTP
  - Reference: <https://openbao.org/docs/secrets/transit/>

**Implementation Location:** `processor/integrityprocessor/`

#### 2. Audit Logging Processor (NEW)

**Purpose:** Log all data modifications for tampering detection

**Features:**

- Log before/after state of data modifications
- Track which processor made changes
- Hash-based change detection
- Configurable log levels (all changes vs. suspicious only)

**Implementation Location:** `processor/auditprocessor/`

#### 3. Data Validation Processor (NEW)

**Purpose:** Validate data structure and content against schemas/rules

**Features:**

- Schema validation (JSON Schema, OTLP schema)
- Attribute whitelisting/blacklisting
- Value range validation
- Pattern matching for suspicious data

**Implementation Location:** `processor/validationprocessor/`

#### 4. Replay Protection Extension (NEW)

**Purpose:** Prevent replay attacks using nonces/timestamps

**Features:**

- Nonce generation and validation
- Timestamp-based replay window (configurable, e.g., 5 minutes)
- Request ID tracking to detect duplicate requests
- Configurable window size and cleanup intervals
- Support for memory-based storage (single instance) or Redis (distributed)
- Header-based nonce/timestamp/request-id extraction
- Automatic cleanup of expired entries
- Configurable maximum requests per window

**Configuration Options:**

- `window`: Time window for replay detection (e.g., 5m, 10m, 1h)
- `nonce_required`: Whether nonce is required in requests
- `request_id_header`: Header name for request ID tracking (default: "X-Request-ID")
- `timestamp_header`: Header name for timestamp (default: "X-Timestamp")
- `nonce_header`: Header name for nonce (default: "X-Nonce")
- `storage_backend`: "memory" or "redis"
- `max_requests_per_window`: Maximum requests to track per window
- `cleanup_interval`: Interval for cleaning up expired entries

**Implementation Location:** `extension/replayprotectionextension/`

#### 5. Enhanced OTLP Receiver with HMAC (ENHANCEMENT)

**Purpose:** Add HMAC verification to standard OTLP receivers

**Features:**

- HMAC signature in headers
- Shared secret configuration (local or OpenBao Transit)
- Signature verification before processing
- Reject unsigned/invalid requests

**Key Management Options:**

- **Local Secret**: Store HMAC secret in environment variable
- **OpenBao Transit**: Use OpenBao Transit API for HMAC verification
  - No secret storage in collector
  - Automatic key rotation support
  - Centralized key management

**Implementation Location:** Enhance `receiver/otlpreceiver/`

#### 6. Processor Authorization Extension (NEW)

**Purpose:** Control which processors can modify which data

**Features:**

- Processor-level permissions
- Attribute-level access control
- Audit trail of authorized modifications
- Policy-based rules

**Implementation Location:** `extension/processorauth extension/`

#### 7. Config Integrity Verification (ENHANCEMENT)

**Purpose:** Verify config file integrity using signatures

**Features:**

- Config file signing
- Signature verification on load
- Reject unsigned configs (optional)
- Support for multiple signers

**Implementation Location:** Enhance config loading in core collector

## Implementation Priority

### High Priority

1. **Enable TLS/mTLS** - Already available, just needs configuration
2. **Data Integrity Processor** - Critical for tampering detection
3. **Enhanced OTLP Receiver with HMAC** - Prevents injection attacks
4. **Audit Logging Processor** - Essential for forensic analysis

### Medium Priority

1. **Replay Protection Extension** - Prevents replay attacks
2. **Data Validation Processor** - Catches accidental tampering
3. **Config Integrity Verification** - Prevents config tampering

### Low Priority

1. **Processor Authorization Extension** - Advanced access control
2. **Queue Encryption** - If persistent queues are used

## Example Secure Configuration

```yaml
extensions:
  oauth2client:
    client_id: ${OAUTH_CLIENT_ID}
    client_secret: ${OAUTH_CLIENT_SECRET}
    token_url: https://auth.example.com/token

  # TODO: When implemented - Replay Protection Extension
  # replayprotection:
  #   window: 5m                    # Time window for replay detection (e.g., 5m, 10m, 1h)
  #   nonce_required: true          # Require nonce in requests
  #   request_id_header: "X-Request-ID"  # Header name for request ID tracking
  #   timestamp_header: "X-Timestamp"    # Header name for timestamp
  #   nonce_header: "X-Nonce"           # Header name for nonce
  #   storage_backend: "memory"          # Storage backend: "memory" or "redis"
  #   # Optional: Redis storage for distributed deployments
  #   # redis:
  #   #   endpoint: redis://localhost:6379
  #   #   password: ${REDIS_PASSWORD}
  #   #   db: 0
  #   #   key_prefix: "otel:replay:"
  #   max_requests_per_window: 1000  # Maximum requests to track per window
  #   cleanup_interval: 1m           # Interval for cleaning up expired entries

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
        # TODO: Add HMAC verification when implemented
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

  # TODO: When implemented - Data Integrity Processor
  # integrity:
  #   # Sign data for downstream verification
  #   sign:
  #     algorithm: HMAC-SHA256        # HMAC algorithm: HMAC-SHA256, HMAC-SHA512
  #     # Option 1: Local secret
  #     secret: ${INTEGRITY_SECRET}
  #     # Option 2: OpenBao Transit (recommended)
  #     # openbao_transit:
  #     #   address: https://openbao.example.com:8200
  #     #   token: ${OPENBAO_TOKEN}
  #     #   key_name: otel-hmac-key
  #     #   mount_path: transit
  #     include_attributes: true      # Include resource/span attributes in signature
  #     signature_header: "X-OTel-Signature"  # Header name for signature
  #   verify: true                    # Verify incoming signatures
  #   verify_header: "X-OTel-Signature"       # Header name to read signature from
  #   reject_invalid: true            # Reject requests with invalid signatures

  # TODO: When implemented - Audit Logging Processor
  # audit:
  #   log_level: info                 # Log level: debug, info, warn, error
  #   log_changes: true               # Log all data modifications
  #   log_unchanged: false            # Log even when no changes detected
  #   output: file:///var/log/otel/audit.log  # Output destination
  #   # Alternative outputs:
  #   # output: stdout
  #   # output: otlp://audit-sink:4317
  #   include_before: true            # Include before state in logs
  #   include_after: true             # Include after state in logs
  #   hash_algorithm: SHA256          # Hash algorithm for change detection
  #   track_processors: true          # Track which processor made changes

  # TODO: When implemented - Data Validation Processor
  # validation:
  #   schema_url: https://opentelemetry.io/schemas/1.0.0  # Schema URL for validation
  #   required_attributes:           # Required resource attributes
  #     - service.name
  #     - service.version
  #   max_attribute_count: 100       # Maximum number of attributes allowed
  #   max_attribute_value_length: 4096  # Maximum length of attribute values
  #   allowed_attribute_keys: []      # Whitelist of allowed attribute keys (empty = all allowed)
  #   blocked_attribute_keys: []     # Blacklist of blocked attribute keys
  #   validate_timestamps: true      # Validate timestamp ranges
  #   reject_invalid: true           # Reject invalid data instead of dropping

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
  # TODO: Add replayprotection to extensions when implemented
  extensions: [oauth2client] # Add replayprotection when implemented
  pipelines:
    traces:
      receivers: [otlp]
      # TODO: Add integrity, audit, validation processors when implemented
      processors: [batch] # Add integrity, audit, validation when implemented
      exporters: [otlp]
    metrics:
      receivers: [otlp]
      # TODO: Add integrity, audit, validation processors when implemented
      processors: [batch] # Add integrity, audit, validation when implemented
      exporters: [otlp]
    logs:
      receivers: [otlp]
      # TODO: Add integrity, audit, validation processors when implemented
      processors: [batch] # Add integrity, audit, validation when implemented
      exporters: [otlp]
```

## References

1. **Secure Tracing Example:** `examples/secure-tracing/README.md`
2. **HMAC Implementation:** `receiver/mongodbatlasreceiver/alerts.go:429-445`
3. **TLS Configuration:** OpenTelemetry Collector TLS documentation
4. **Authentication Extensions:** Various extensions in `extension/` directory
5. **OpenBao Transit:** <https://openbao.org/docs/secrets/transit/> (for centralized HMAC key management)
