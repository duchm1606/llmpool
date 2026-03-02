# Session 7: Device Flow + Security

**Duration**: 60 minutes (20 concept / 35 coding / 5 recap)  
**Session File**: `session-7-device-security.md`  
**Evidence Directory**: `evidence/session-7/`

---

## Objective

Implement OAuth device flow endpoints and add security guardrails. This session enables CLI/headless authentication and hardens the OAuth implementation against common attacks.

- [ ] Implement device code endpoint (`/codex-device-code`)
- [ ] Implement device status polling endpoint (`/device-status`)
- [ ] Add rate limiting for OAuth endpoints
- [ ] Implement state validation security rules
- [ ] Add audit logging for OAuth events
- [ ] Create security middleware for management endpoints

---

## Prerequisites

Before starting this session, ensure:

- [ ] Previous session completed: Session 6 (Refresh Contract + Worker)
- [ ] Reference files open:
  - `.sisyphus/docs/oauth-contract-map.md` - Section 1.2 (Device Code Flow)
  - `.sisyphus/docs/oauth-contract-map.md` - Section 9 (Security Considerations)
  - `internal/infra/oauth/state.go` - State validation
- [ ] Environment ready:
  - `docker compose up -d` running (postgres + redis)
  - `go test ./...` passes in current state
- [ ] Knowledge of:
  - OAuth 2.0 Device Authorization Grant (RFC 8628)
  - Rate limiting strategies
  - Security headers and CSRF protection

---

## Concept Review (20 min)

### Key Concepts

Brief explanation of concepts to understand before coding.

1. **Device Flow**: For input-constrained devices (CLI, smart TVs). User visits URL on another device, enters code, authorizes. Client polls for completion.

2. **Device Code Flow Steps**:
   - Client requests device code from provider
   - Provider returns: device_code, user_code, verification_uri, expires_in, interval
   - Client displays user_code and verification_uri to user
   - User visits verification_uri, enters user_code, authorizes
   - Client polls token endpoint with device_code
   - Provider responds with tokens when authorized

3. **Rate Limiting**: Prevent abuse of OAuth endpoints. Different limits for different endpoints (auth URL vs callback vs status).

4. **Security Guardrails**:
   - Strict state validation (size, charset, no traversal)
   - PKCE verifier storage (never expose)
   - Log redaction (no tokens in logs)
   - Request timeouts
   - IP-based rate limiting

### Architecture Placement

Where this code fits in the Clean Architecture:

```
internal/
  delivery/
    http/
      handler/
        oauth_handler.go      <- Add DeviceCode and DeviceStatus (you are here)
      middleware/
        security.go           <- Rate limiting, auth checks
  infra/
    oauth/
      device_flow.go          <- Device flow implementation
    security/
      rate_limiter.go         <- Rate limiting logic
      audit_logger.go         <- Audit event logging
```

### Interface Contracts

```go
// DeviceCodeResponse - response from device code endpoint
type DeviceCodeResponse struct {
    Status          string `json:"status"`            // Always "ok"
    VerificationURI string `json:"verification_uri"`  // URL for user to visit
    VerificationURL string `json:"verification_url"`  // Alias for compatibility
    URL             string `json:"url"`               // Alias for compatibility
    UserCode        string `json:"user_code"`         // Code user enters
    State           string `json:"state"`             // Device code for polling
    DeviceCode      string `json:"device_code"`       // Alias for state
    ExpiresIn       int    `json:"expires_in"`        // Seconds until expiry
    Interval        int    `json:"interval"`          // Seconds between polls
}

// DeviceStatusResponse - response from device status endpoint
type DeviceStatusResponse struct {
    Status string `json:"status"`           // "wait", "ok", or "error"
    Error  string `json:"error,omitempty"`  // Error message if status=error
}

// DeviceFlowService interface
type DeviceFlowService interface {
    InitiateDeviceFlow(ctx context.Context) (*DeviceCodeResponse, error)
    PollDeviceStatus(ctx context.Context, deviceCode string) (*DeviceStatusResponse, error)
}

// RateLimiter interface
type RateLimiter interface {
    Allow(key string) bool                    // Check if request allowed
    AllowN(key string, n int) bool            // Check N requests
    RetryAfter(key string) time.Duration      // Time until next allowed request
}
```

### Device Flow Response Codes

| Endpoint | Status | Body | Description |
|----------|--------|------|-------------|
| Device Code | 200 | DeviceCodeResponse | Codes generated successfully |
| Device Code | 429 | Error | Rate limit exceeded |
| Device Status | 200 | DeviceStatusResponse | Current status |
| Device Status | 404 | Error | Unknown device code |
| Device Status | 429 | Error | Polling too fast |

---

## RED Phase: Write Failing Tests (10 min)

### Test Cases to Write

- [ ] `TestDeviceCode_Success` - Returns all required fields
- [ ] `TestDeviceCode_RateLimited` - Returns 429 when limit exceeded
- [ ] `TestDeviceStatus_Wait` - Returns wait when not yet authorized
- [ ] `TestDeviceStatus_Ok` - Returns ok when authorized
- [ ] `TestDeviceStatus_Error` - Returns error on authorization failure
- [ ] `TestDeviceStatus_PollingInterval` - Returns 429 when polling too fast
- [ ] `TestSecurityMiddleware_MissingManagementKey` - Rejects requests without X-Management-Key
- [ ] `TestStateValidation_SecurityRules` - Validates all security constraints

### Test File Location

`internal/delivery/http/handler/oauth_device_test.go`  
`internal/delivery/http/middleware/security_test.go`

### Expected Test Output (Save as Evidence)

Run tests to confirm they fail as expected:

```bash
cd /Users/duchoang/Projects/llmpool-worktrees/codex-pkce-learning-sessions
go test -v ./internal/delivery/http/handler -run "TestDevice" 2>&1 | tee .sisyphus/learning/evidence/session-7-red.txt
go test -v ./internal/delivery/http/middleware -run "TestSecurity" 2>&1 | tee -a .sisyphus/learning/evidence/session-7-red.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-7-red.txt`

---

## GREEN Phase: Minimum Implementation (15 min)

### Implementation Steps

#### 1. Device Flow Service

Create/extend `internal/usecase/oauth/device_flow.go`:

- [ ] InitiateDeviceFlow - calls provider device endpoint
- [ ] Parse response into DeviceCodeResponse
- [ ] Store device session in Redis with TTL
- [ ] PollDeviceStatus - check if tokens received
- [ ] Return appropriate status based on session state

#### 2. Handler Methods

Extend `internal/delivery/http/handler/oauth_handler.go`:

- [ ] HandleDeviceCode - GET /v1/internal/oauth/codex-device-code
- [ ] HandleDeviceStatus - GET /v1/internal/oauth/device-status
- [ ] Return Proxypal-compatible field aliases

#### 3. Rate Limiter

Create `internal/infra/security/rate_limiter.go`:

- [ ] Redis-backed rate limiter using sliding window
- [ ] Different limits per endpoint:
  - Auth URL: 10/min per IP
  - Callback: 100/min per IP
  - Status poll: 60/min per state
  - Device code: 5/min per IP

#### 4. Security Middleware

Create `internal/delivery/http/middleware/security.go`:

- [ ] ManagementKey middleware - validates X-Management-Key header
- [ ] RateLimit middleware - applies rate limiting
- [ ] AuditLog middleware - logs OAuth events (redacted)

### Implementation Locations

- `internal/usecase/oauth/device_flow.go` - Service logic
- `internal/delivery/http/handler/oauth_handler.go` - HTTP handlers
- `internal/infra/security/rate_limiter.go` - Rate limiting
- `internal/delivery/http/middleware/security.go` - Security middleware

### Verification Commands

```bash
# Run specific tests
go test -v ./internal/delivery/http/handler -run "TestDevice"
go test -v ./internal/delivery/http/middleware -run "TestSecurity"
go test -v ./internal/infra/security

# Save passing output as evidence
go test -v ./internal/delivery/http/handler ./internal/delivery/http/middleware ./internal/infra/security 2>&1 | tee .sisyphus/learning/evidence/session-7-green.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-7-green.txt`

---

## REFACTOR Phase: Clean Up (10 min)

### Refactoring Checklist

- [ ] Extract rate limit configuration to config file
- [ ] Add rate limit headers (X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset)
- [ ] Implement token bucket algorithm for burst handling
- [ ] Add IP whitelist for internal services
- [ ] Create security event types for audit log
- [ ] Add structured context to audit events (request ID, user agent)
- [ ] Document all security decisions in code comments

### Refactoring Evidence

After refactoring, verify tests still pass:

```bash
go test ./internal/delivery/http/handler ./internal/delivery/http/middleware ./internal/infra/security 2>&1 | tee .sisyphus/learning/evidence/session-7-refactor.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-7-refactor.txt`

---

## Recap (5 min)

### What Was Built

Summary of the implementation completed.

1. **Device Code Endpoint** - Initiates device flow, returns user_code and verification_uri
2. **Device Status Endpoint** - Polls for authorization completion
3. **Rate Limiter** - Redis-backed sliding window rate limiting
4. **Security Middleware** - Management key validation and audit logging
5. **Security Guardrails** - State validation, log redaction, request timeouts

### Key Decisions

- Decision 1: Device flow uses same session store as web flow (unified abstraction)
- Decision 2: Rate limits stricter for device code (5/min) than auth URL (10/min)
- Decision 3: Audit logs never contain tokens or codes (security)
- Decision 4: Management key checked before rate limiting (fail fast)

### Test Coverage

| Test | Status | Evidence |
|------|--------|----------|
| `TestDeviceCode_Success` | PASS/FAIL | `session-7-green.txt` |
| `TestDeviceCode_RateLimited` | PASS/FAIL | `session-7-green.txt` |
| `TestDeviceStatus_Wait` | PASS/FAIL | `session-7-green.txt` |
| `TestSecurityMiddleware_MissingManagementKey` | PASS/FAIL | `session-7-green.txt` |
| `TestStateValidation_SecurityRules` | PASS/FAIL | `session-7-green.txt` |

### Links to Evidence

- RED: `.sisyphus/learning/evidence/session-7-red.txt`
- GREEN: `.sisyphus/learning/evidence/session-7-green.txt`
- REFACTOR: `.sisyphus/learning/evidence/session-7-refactor.txt`
- API Response: `.sisyphus/learning/evidence/session-7-device-code.json`

### Next Session Preview

Session 8: Integration + Capstone - Full integration tests and final verification

---

## Verify (Mandatory)

Before marking this session complete, run all verification commands:

### Unit Tests

```bash
# Run all tests in modified packages
go test -v ./internal/delivery/http/handler
go test -v ./internal/delivery/http/middleware
go test -v ./internal/infra/security
go test -v ./internal/usecase/oauth
```

### Integration Tests (if applicable)

```bash
# Start services if needed
docker compose up -d

# Test device code endpoint
curl -s http://localhost:8080/v1/internal/oauth/codex-device-code \
  -H "X-Management-Key: test-key" | jq

# Test rate limiting (should fail after limit)
for i in {1..15}; do
  curl -s -o /dev/null -w "%{http_code}\n" \
    http://localhost:8080/v1/internal/oauth/codex-device-code \
    -H "X-Management-Key: test-key"
done
```

### Lint

```bash
make lint
```

### Full Suite

```bash
# Must pass before session is complete
go test ./...
make lint
```

### Evidence Collection

Save final verification output:

```bash
go test ./... 2>&1 | tee .sisyphus/learning/evidence/session-7-verify.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-7-verify.txt`

---

## Session Complete Checklist

- [ ] All RED tests written and failing
- [ ] GREEN phase: minimum implementation passes tests
- [ ] REFACTOR phase: code cleaned, tests still pass
- [ ] Evidence files saved in `.sisyphus/learning/evidence/`
- [ ] Full test suite passes: `go test ./...`
- [ ] Lint passes: `make lint`
- [ ] Session notes updated
- [ ] Next session prerequisites noted
