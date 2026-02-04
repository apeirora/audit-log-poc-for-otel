# Tampering Resistance Summary

## Architecture Overview

```text
Application → Sidecar Collector → OTEL Collector → Sink
```

## 1. Vulnerabilities and Their Locations

### 1.1 Network Transmission Vulnerabilities

#### Application → Sidecar Collector

**Location:** Network layer between application and sidecar collector **Vulnerabilities:**

- **MITM Attacks** - Man-in-the-middle interception and modification
- **Packet Injection** - Malicious data injection into the stream
- **Replay Attacks** - Replaying captured legitimate requests
- **Network Misconfiguration** - Accidental routing/proxy issues
- **Packet Corruption** - Data corruption during transmission

#### Sidecar Collector → OTEL Collector

**Location:** Network layer between sidecar and main collector **Vulnerabilities:**

- All vulnerabilities from 1.1, plus:
- **Accidental Configuration Misconfiguration** - Misconfigured sidecar causing data issues

#### OTEL Collector → Sink

**Location:** Network layer between collector and destination sink **Vulnerabilities:**

- **MITM Attacks** - Interception of outbound connections
- **Data Replay Requests** - Sink requesting data replay
- **Unauthorized Data Access** - Unauthorized access at sink endpoint

### 1.2 Collector Internal Processing Vulnerabilities

#### Receiver Processing

**Location:** `receiver/` components (especially `receiver/otlpreceiver/`) **Vulnerabilities:**

- **Receiver Bugs** - Accidental data corruption from receiver bugs
- **Unauthorized Data Injection** - Injection via receiver endpoints (network-based attacks)
- **Missing HMAC Verification** - No integrity checks for standard OTLP receivers
- **No Data Validation** - No receiver-level whitelisting/validation
- **Limited Rate Limiting** - Only available in specific receivers (e.g., `yanggrpcreceiver`)

#### Processor Pipeline

**Location:** `processor/` components in the processing pipeline **Vulnerabilities:**

- **Misconfigured Processors** - Accidental data loss (e.g., `filterprocessor`)
- **Processor Bugs** - Accidental data corruption from processor bugs
- **No Audit Logging** - No tracking of processor modifications
- **No Integrity Checks** - No verification between processors
- **No Change Detection** - Cannot detect unexpected modifications
- **No Order Validation** - No validation of processor execution order

#### Exporter Processing

**Location:** `exporter/` components **Vulnerabilities:**

- **Exporter Bugs** - Accidental data corruption
- **Unauthorized Data Export** - Unauthorized data export to unauthorized sinks (via network misconfiguration)
- **No Integrity Verification** - No exporter-level data integrity checks
- **No Audit Trail** - No logging of exported data

### 1.3 Configuration and Runtime Vulnerabilities

#### Configuration Tampering

**Location:** Configuration loading and management (`cmd/otelcorecol/` and config loading) **Vulnerabilities:**

- **Misconfiguration** - Accidental config errors causing data loss/modification
- **No Integrity Verification** - No config file signature verification (for externally stored configs)
- **No Audit Logging** - No tracking of config changes

#### Runtime Environment

**Location:** Collector runtime process **Vulnerabilities:**

- **Resource Exhaustion** - Accidental resource limits causing data loss

### 1.4 Data Storage Vulnerabilities

#### Queues/Buffers

**Location:** Persistent queue storage (if enabled in exporters) **Vulnerabilities:**

- **Queue Corruption** - Accidental queue data corruption
- **No Integrity Verification** - No queue data integrity checks (for detecting corruption)
- **No Encryption** - Unencrypted queue storage (for data at rest protection)

## 2. What is Already Implemented vs What Needs Implementation

### 2.1 Already Implemented ✅

#### Network Security

- **TLS/mTLS Support**
  - Location: OTLP receivers and exporters
  - Reference: `examples/secure-tracing/README.md`
  - Status: Fully implemented, needs configuration

- **Authentication Extensions**
  - `basicauthextension` - Basic authentication
  - `bearertokenauthextension` - Bearer token authentication
  - `oidcauthextension` - OIDC authentication
  - `oauth2clientauthextension` - OAuth2 client credentials
  - Location: `extension/` directory
  - Status: Fully implemented

- **OTLP Arrow Receiver Authentication**
  - Location: `receiver/otelarrowreceiver/internal/arrow/arrow.go:575-582`
  - Status: Implemented

- **Exporter Authentication**
  - Bearer tokens (e.g., `tinybirdexporter`, `sumologicexporter`)
  - API keys (e.g., `coralogixexporter`)
  - AWS SigV4 (`sigv4authextension`)
  - Status: Implemented in various exporters

#### Receiver Security

- **HMAC Signature Verification**
  - Location: `receiver/mongodbatlasreceiver/alerts.go:299-303`
  - Implementation: HMAC-SHA1 for payload verification
  - Status: Implemented for MongoDB Atlas receiver only

- **Content-Length Validation**
  - Status: Implemented in receivers

- **Rate Limiting**
  - Location: `yanggrpcreceiver` (and some other specific receivers)
  - Status: Implemented in specific receivers only

#### Processor Security

- **Schema Processor**
  - Location: `processor/schemaprocessor/`
  - Status: Implemented, validates data structure

- **Filter Processor**
  - Status: Implemented, can filter data based on rules

- **Isolation Forest Processor**
  - Location: `processor/isolationforestprocessor/`
  - Status: Implemented (alpha), provides unsupervised anomaly detection
  - Features: Detects behavioral anomalies in traces, metrics, and logs using Isolation Forest algorithm
  - Use case: Can detect unusual patterns that may indicate tampering, injection attacks, or unexpected data modifications
  - Reference: `processor/isolationforestprocessor/README.md`

- **Explicit Processor Configuration**
  - Status: Processors are configuration-driven and visible

#### Configuration Security

- **Config Validation**
  - Status: Collector validates config on startup

- **OPAMP Extension**
  - Status: Remote config management with validation

#### Runtime Security

- **Health Check Extensions**
  - Status: Monitor collector health

- **Observability**
  - Status: Collector emits its own telemetry

- **Resource Management**
  - Status: Basic resource limits and monitoring

#### Storage Security

- **Persistent Queues**
  - Status: Some exporters support persistent storage

### 2.2 Needs Implementation ❌

#### Network Security Gaps

- **Data Integrity Verification (HMAC/Signatures) for OTLP**
  - Status: Not implemented for standard OTLP receivers
  - Needed: HMAC verification in `receiver/otlpreceiver/`

- **Replay Attack Protection**
  - Status: Not implemented
  - Needed: Nonce/timestamp validation extension

- **Fine-grained Authorization**
  - Status: Only authentication available, not authorization
  - Needed: Authorization extension

- **End-to-end Data Integrity**
  - Status: No end-to-end verification
  - Needed: Integrity processor

- **Proof of Origin**
  - Status: No origin verification
  - Needed: Signature-based origin verification

#### Receiver Security Gaps

- **HMAC Verification for Standard OTLP Receivers**
  - Status: Only in `mongodbatlasreceiver`
  - Needed: Enhance `receiver/otlpreceiver/` with HMAC

- **Receiver-level Data Validation/Whitelisting**
  - Status: Not implemented
  - Needed: Validation processor or receiver enhancement

- **Rate Limiting at Receiver Level**
  - Status: Only in specific receivers
  - Needed: Universal rate limiting extension

#### Processor Security Gaps

- **Audit Logging of Processor Modifications**
  - Status: Not implemented
  - Needed: `processor/auditprocessor/`

- **Integrity Checks Between Processors**
  - Status: Not implemented
  - Needed: Integrity processor

- **Detection of Unexpected Data Modifications**
  - Status: Not implemented
  - Needed: Audit processor with change detection

- **Processor Execution Order Validation**
  - Status: Not implemented
  - Needed: Validation logic

#### Exporter Security Gaps

- **Exporter-level Data Integrity Verification**
  - Status: Not implemented
  - Needed: Integrity processor in export pipeline

- **Audit Trail of Exported Data**
  - Status: Not implemented
  - Needed: Audit processor

#### Configuration Security Gaps

- **Config File Integrity Verification (Signatures)**
  - Status: Not implemented
  - Needed: Enhance config loading in core collector (for externally stored configs)

- **Config Change Audit Logging**
  - Status: Not implemented
  - Needed: Config change tracking

#### Storage Security Gaps

- **Queue Data Integrity Verification**
  - Status: Not implemented
  - Needed: Queue integrity checks (for detecting accidental corruption)

- **Encrypted Queue Storage**
  - Status: Not implemented
  - Needed: Queue encryption (for data at rest protection)

## 3. Implementation Todos with Options

### Priority 1: High Priority

#### TODO 1: Data Integrity Processor

**Purpose:** Add HMAC signatures to telemetry data for end-to-end integrity verification

**Location:** `processor/integrityprocessor/`

**Features to Implement:**

- Sign data at source (application/sidecar)
- Verify signatures at collector
- Re-sign for downstream (collector → sink)
- Support multiple algorithms (HMAC-SHA256, HMAC-SHA512)

**Implementation Options:**

##### Option A: Local Secret Storage (Simple)

- Store HMAC secrets in environment variables or config files
- Pros: Simple, no external dependencies
- Cons: Secrets in config, manual rotation, no centralized management
- Implementation:
  - Read secret from env var or config
  - Use Go `crypto/hmac` package
  - Add signature to resource/span attributes or headers

##### Option B: OpenBao Transit (Recommended)

- Use OpenBao Transit secrets engine for centralized key management
- Pros: Centralized management, automatic rotation, no secrets in config, audit logging
- Cons: Requires OpenBao infrastructure
- Implementation:
  - HTTP client for OpenBao Transit API
  - Use `/v1/transit/hmac/{key_name}` endpoint
  - Reference: <https://openbao.org/docs/secrets/transit/>
  - Support both signing and verification via API

**Reference Implementation:** `receiver/mongodbatlasreceiver/alerts.go:429-445`

---

#### TODO 2: Enhanced OTLP Receiver with HMAC

**Purpose:** Add HMAC verification to standard OTLP receivers

**Location:** Enhance `receiver/otlpreceiver/`

**Features to Implement:**

- HMAC signature in headers (`X-OTel-HMAC-Signature` or configurable)
- Shared secret configuration (local or OpenBao Transit)
- Signature verification before processing
- Reject unsigned/invalid requests

**Implementation Options:**

##### Option A: Local Secret

- Store HMAC secret in environment variable
- Pros: Simple, fast
- Cons: Secret management overhead
- Implementation:
  - Add HMAC config section to receiver config
  - Extract signature from headers
  - Verify using Go `crypto/hmac`
  - Reject if invalid or missing (if required)

##### Option B: OpenBao Transit

- Use OpenBao Transit API for HMAC verification
- Pros: No secret storage, automatic rotation, centralized
- Cons: Network dependency, potential latency
- Implementation:
  - HTTP client for OpenBao Transit
  - Use `/v1/transit/verify/{key_name}` endpoint
  - Cache verification results if needed

**Reference Implementation:** `receiver/mongodbatlasreceiver/alerts.go:284-303`

---

#### TODO 3: Audit Logging Processor

**Purpose:** Log all data modifications for tampering detection and forensic analysis

**Location:** `processor/auditprocessor/`

**Features to Implement:**

- Log before/after state of data modifications
- Track which processor made changes
- Hash-based change detection
- Configurable log levels (all changes vs. suspicious only)
- Multiple output destinations (file, stdout, OTLP)

**Implementation Options:**

##### Option A: File-based Logging

- Write audit logs to file system
- Pros: Simple, persistent
- Cons: File management, rotation needed
- Implementation:
  - Use Go `log` or structured logging
  - JSON format for structured logs
  - Include processor name, timestamp, before/after hashes
  - Support log rotation

##### Option B: OTLP Export

- Export audit logs as telemetry
- Pros: Centralized, queryable
- Cons: Requires separate pipeline
- Implementation:
  - Create separate OTLP exporter for audit logs
  - Send as logs or traces
  - Include metadata about changes

##### Option C: Hybrid Approach (Audit Logging)

- Support both file and OTLP export
- Pros: Flexible, redundant
- Cons: More complex configuration
- Implementation:
  - Configurable output destinations
  - Support multiple outputs simultaneously

---

### Priority 2: Medium Priority

#### TODO 4: Replay Protection Extension

**Purpose:** Prevent replay attacks using nonces/timestamps

**Location:** `extension/replayprotectionextension/`

**Features to Implement:**

- Nonce generation and validation
- Timestamp-based replay window (configurable, e.g., 5 minutes)
- Request ID tracking to detect duplicate requests
- Configurable window size and cleanup intervals
- Support for memory-based storage (single instance) or Redis (distributed)
- Header-based nonce/timestamp/request-id extraction
- Automatic cleanup of expired entries
- Configurable maximum requests per window

**Implementation Options:**

##### Option A: Memory-based Storage

- Store nonces/request IDs in in-memory map
- Pros: Fast, no external dependencies
- Cons: Not distributed, lost on restart
- Implementation:
  - Go `sync.Map` or `map[string]time.Time` with mutex
  - TTL-based expiration
  - Background goroutine for cleanup
  - Key: request_id or nonce, Value: timestamp

##### Option B: Redis Storage

- Use Redis for distributed storage
- Pros: Distributed, persistent, scalable
- Cons: Requires Redis infrastructure
- Implementation:
  - Redis client (e.g., `github.com/redis/go-redis/v9`)
  - Use Redis TTL for automatic expiration
  - Key prefix: `otel:replay:{request_id}`
  - Use Redis SET with NX (only if not exists) for atomic operations

##### Option C: Hybrid Approach (Replay Protection)

- Support both memory and Redis
- Pros: Flexible deployment
- Cons: More complex code
- Implementation:
  - Interface for storage backend
  - Memory and Redis implementations
  - Configurable backend selection

**Configuration Options:**

- `window`: Time window (e.g., 5m, 10m, 1h)
- `nonce_required`: Whether nonce is required
- `request_id_header`: Header name (default: "X-Request-ID")
- `timestamp_header`: Header name (default: "X-Timestamp")
- `nonce_header`: Header name (default: "X-Nonce")
- `storage_backend`: "memory" or "redis"
- `max_requests_per_window`: Maximum requests to track
- `cleanup_interval`: Cleanup interval

---

#### TODO 5: Data Validation Processor

**Purpose:** Validate data structure and content against schemas/rules

**Location:** `processor/validationprocessor/`

**Features to Implement:**

- Schema validation (JSON Schema, OTLP schema)
- Attribute whitelisting/blacklisting
- Value range validation
- Pattern matching for suspicious data
- Timestamp validation
- Maximum attribute count/length validation

**Implementation Options:**

##### Option A: JSON Schema Validation

- Use JSON Schema for validation
- Pros: Standard, flexible
- Cons: Requires schema definitions
- Implementation:
  - Use `github.com/xeipuuv/gojsonschema`
  - Load schema from URL or file
  - Validate OTLP data converted to JSON
  - Report validation errors

##### Option B: OTLP Schema Validation

- Use OpenTelemetry schema definitions
- Pros: Native OTLP support
- Cons: Limited to OTLP schemas
- Implementation:
  - Use OpenTelemetry schema registry
  - Validate against semantic conventions
  - Check required attributes

##### Option C: Rule-based Validation

- Custom validation rules
- Pros: Flexible, simple
- Cons: Manual rule definition
- Implementation:
  - Configurable rules (YAML/JSON)
  - Pattern matching for attributes
  - Range validation for numeric values
  - Regex for string patterns

##### Option D: Hybrid Approach

- Support schema and rule-based validation
- Pros: Most flexible
- Cons: More complex
- Implementation:
  - Support both validation methods
  - Combine results
  - Configurable validation mode

---

#### TODO 6: Config Integrity Verification

**Purpose:** Verify config file integrity using signatures (for externally stored configs)

**Location:** Enhance config loading in core collector (`cmd/otelcorecol/` and config package)

**Features to Implement:**

- Config file signing
- Signature verification on load
- Reject unsigned configs (optional)
- Support for multiple signers

**Implementation Options:**

##### Option A: GPG Signatures

- Use GPG for config signing/verification
- Pros: Standard tooling, well-established
- Cons: Requires GPG infrastructure
- Implementation:
  - Use `golang.org/x/crypto/openpgp`
  - Verify signature on config load
  - Support detached signatures (.sig files)
  - Configurable public key location

##### Option B: X.509 Certificates

- Use X.509 certificate-based signatures
- Pros: PKI integration, enterprise-friendly
- Cons: Requires certificate management
- Implementation:
  - Use `crypto/x509` and `crypto/ecdsa` or `crypto/rsa`
  - Sign config with private key
  - Verify with public certificate
  - Support certificate chains

##### Option C: HMAC Signatures

- Use HMAC for config integrity
- Pros: Simple, fast
- Cons: Symmetric key management
- Implementation:
  - Similar to data integrity processor
  - Sign config file content
  - Verify on load
  - Store signature in separate file or metadata

---

### Priority 3: Low Priority

#### TODO 7: Queue Encryption

**Purpose:** Encrypt queue storage for persistent queues

**Location:** Enhance queue storage in exporters (e.g., `exporter/` components with persistent queues)

**Features to Implement:**

- Encrypt queue data at rest
- Decrypt on read
- Key management integration

**Implementation Options:**

##### Option A: AES Encryption

- Use AES-256-GCM for encryption
- Pros: Standard, secure
- Cons: Key management needed
- Implementation:
  - Use `crypto/aes` and `crypto/cipher`
  - Encrypt before writing to queue
  - Decrypt after reading
  - Store encryption key securely (env var or key management)

##### Option B: OpenBao Transit Encryption

- Use OpenBao Transit for encryption
- Pros: Centralized key management
- Cons: Network dependency
- Implementation:
  - Use OpenBao Transit encrypt/decrypt API
  - Encrypt data before queue write
  - Decrypt after queue read
  - Reference: <https://openbao.org/docs/secrets/transit/>

##### Option C: File System Encryption

- Rely on encrypted file system
- Pros: Transparent, OS-level
- Cons: Requires encrypted filesystem setup
- Implementation:
  - Store queue on encrypted volume
  - No code changes needed
  - Document requirements

---

## Implementation Priority Summary

### High Priority (Immediate)

1. **Data Integrity Processor** - Critical for tampering detection
2. **Enhanced OTLP Receiver with HMAC** - Prevents injection attacks
3. **Audit Logging Processor** - Essential for forensic analysis

### Medium Priority (Short-term)

1. **Replay Protection Extension** - Prevents replay attacks
2. **Data Validation Processor** - Catches accidental tampering
3. **Config Integrity Verification** - Prevents config tampering (for externally stored configs)

### Low Priority (Long-term)

1. **Queue Encryption** - If persistent queues are used (for data at rest protection)

## References

1. **Secure Tracing Example:** `examples/secure-tracing/README.md`
2. **HMAC Implementation:** `receiver/mongodbatlasreceiver/alerts.go:429-445`
3. **Schema Processor:** `processor/schemaprocessor/README.md`
4. **Authentication Extensions:** `extension/` directory
5. **OpenBao Transit:** <https://openbao.org/docs/secrets/transit/> (for centralized HMAC key management)
6. **OTLP Arrow Receiver:** `receiver/otelarrowreceiver/internal/arrow/arrow.go:575-582`
