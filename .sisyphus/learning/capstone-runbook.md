# Capstone Runbook: OAuth PKCE Implementation

Complete verification guide for the llmpool OAuth PKCE implementation.

---

## Overview

This runbook provides exact commands to verify the entire OAuth implementation, expected outputs, and solutions for common issues.

**Scope**: Sessions 1-8, Tasks T1-T20  
**Target Environment**: Docker Compose with PostgreSQL + Redis  
**Estimated Time**: 15 minutes for full verification

---

## Prerequisites

Before running verification:

```bash
# 1. Navigate to project directory
cd /Users/duchoang/Projects/llmpool-worktrees/codex-pkce-learning-sessions

# 2. Start dependencies
docker compose up -d postgres redis

# 3. Verify services are healthy
docker compose ps
```

Expected output:
```
NAME                IMAGE                STATUS
postgres            postgres:15          Up 10 seconds (healthy)
redis               redis:7-alpine       Up 10 seconds (healthy)
```

---

## Phase 1: Environment Verification

### 1.1 Check Environment Variables

```bash
# Required environment variables
export LLMPOOL_ENCRYPTION_KEY="test-encryption-key-32-bytes-long"
export LLMPOOL_OAUTH_CODEX_CLIENT_ID="test-client-id"
export LLMPOOL_OAUTH_CODEX_CLIENT_SECRET="test-client-secret"
export LLMPOOL_MANAGEMENT_KEY="test-management-key"

# Verify they are set
echo "Encryption key: ${LLMPOOL_ENCRYPTION_KEY:0:10}..."
echo "Management key: ${LLMPOOL_MANAGEMENT_KEY}"
```

Expected: Values displayed (truncated for security)

### 1.2 Database Connectivity

```bash
# Test PostgreSQL connection
docker compose exec postgres psql -U postgres -d llmpool -c "SELECT 1;"

# Test Redis connection
docker compose exec redis redis-cli ping
```

Expected outputs:
```
 ?column?
----------
        1
(1 row)

PONG
```

---

## Phase 2: Build Verification

### 2.1 Compile Application

```bash
# Build the binary
go build -o bin/api ./cmd/api

# Verify binary exists
ls -la bin/api
```

Expected: Binary created without errors

### 2.2 Run Unit Tests

```bash
# Run all unit tests
go test ./internal/... -v 2>&1 | tee /tmp/unit-tests.log

# Check for failures
grep -E "(FAIL|PASS|ok|---)" /tmp/unit-tests.log | tail -20
```

Expected output pattern:
```
=== RUN   TestName
--- PASS: TestName (0.00s)
PASS
ok      github.com/duchoang/llmpool/internal/xxx     0.123s
```

**Success Criteria**: All tests PASS, no FAIL lines

### 2.3 Run Linter

```bash
# Run golangci-lint
make lint 2>&1 | tee /tmp/lint.log

# Check for issues
echo "Exit code: $?"
```

Expected: Exit code 0, no issues reported

---

## Phase 3: Integration Verification

### 3.1 Start Full Application

```bash
# Start application with all dependencies
docker compose up -d

# Wait for startup
sleep 5

# Check application health
curl -s http://localhost:8080/health | jq
```

Expected output:
```json
{
  "status": "ok"
}
```

### 3.2 Test Auth URL Endpoint

```bash
# Test native endpoint
curl -s "http://localhost:8080/v1/internal/oauth/codex-auth-url" \
  -H "X-Management-Key: test-management-key" | jq

# Test compatibility alias
curl -s "http://localhost:8080/v0/management/codex-auth-url" \
  -H "X-Management-Key: test-management-key" | jq
```

Expected output:
```json
{
  "status": "ok",
  "url": "https://auth.openai.com/authorize?...",
  "state": "xxx..."
}
```

**Verify**:
- Status is "ok"
- URL contains code_challenge
- State is present (43 characters)

### 3.3 Test Device Code Endpoint

```bash
# Request device code
curl -s "http://localhost:8080/v1/internal/oauth/codex-device-code" \
  -H "X-Management-Key: test-management-key" | jq
```

Expected output:
```json
{
  "status": "ok",
  "verification_uri": "https://auth.openai.com/device",
  "user_code": "XXXX-XXXX",
  "state": "device-xxx...",
  "expires_in": 900,
  "interval": 5
}
```

**Verify**:
- All required fields present
- verification_uri is valid URL
- user_code matches format XXXX-XXXX

### 3.4 Test Callback Endpoint

```bash
# Test callback with invalid state (should return 404)
curl -s -w "\nHTTP Status: %{http_code}\n" \
  -X POST "http://localhost:8080/v1/internal/oauth/callback" \
  -H "Content-Type: application/json" \
  -H "X-Management-Key: test-management-key" \
  -d '{
    "provider": "codex",
    "code": "test-code",
    "state": "invalid-state"
  }'
```

Expected output:
```
{"status":"error","error":"unknown or expired state"}
HTTP Status: 404
```

### 3.5 Test Status Endpoint

```bash
# Test status with state from auth URL response
STATE=$(curl -s "http://localhost:8080/v1/internal/oauth/codex-auth-url" \
  -H "X-Management-Key: test-management-key" | jq -r '.state')

curl -s "http://localhost:8080/v1/internal/oauth/status?state=${STATE}" \
  -H "X-Management-Key: test-management-key" | jq
```

Expected output:
```json
{
  "status": "wait"
}
```

---

## Phase 4: Security Verification

### 4.1 Test Missing Management Key

```bash
# Request without management key
curl -s -w "\nHTTP Status: %{http_code}\n" \
  "http://localhost:8080/v1/internal/oauth/codex-auth-url"
```

Expected: HTTP Status: 401 or 403

### 4.2 Test Invalid Management Key

```bash
# Request with wrong management key
curl -s -w "\nHTTP Status: %{http_code}\n" \
  -H "X-Management-Key: wrong-key" \
  "http://localhost:8080/v1/internal/oauth/codex-auth-url"
```

Expected: HTTP Status: 401 or 403

### 4.3 Test Rate Limiting

```bash
# Rapid requests to trigger rate limit
for i in {1..15}; do
  curl -s -o /dev/null -w "%{http_code}\n" \
    "http://localhost:8080/v1/internal/oauth/codex-auth-url" \
    -H "X-Management-Key: test-management-key"
done
```

Expected: First ~10 requests return 200, subsequent return 429

### 4.4 Test State Validation

```bash
# Test with invalid state format
curl -s -w "\nHTTP Status: %{http_code}\n" \
  -X POST "http://localhost:8080/v1/internal/oauth/callback" \
  -H "Content-Type: application/json" \
  -H "X-Management-Key: test-management-key" \
  -d '{
    "provider": "codex",
    "code": "test",
    "state": "../../../etc/passwd"
  }'
```

Expected: HTTP Status: 400 (invalid state format)

---

## Phase 5: Full Integration Test

### 5.1 Run Integration Test Suite

```bash
# Run all integration tests
go test ./tests/integration/... -v 2>&1 | tee /tmp/integration-tests.log

# Check results
grep -E "(PASS|FAIL)" /tmp/integration-tests.log
```

Expected: All tests PASS

### 5.2 Run Regression Tests

```bash
# Run regression suite
go test ./tests/regression/... -v 2>&1 | tee /tmp/regression-tests.log

# Check results
grep -E "(PASS|FAIL)" /tmp/regression-tests.log
```

Expected: All tests PASS

### 5.3 Run Contract Tests

```bash
# Run contract tests
go test ./tests/contract/... -v 2>&1 | tee /tmp/contract-tests.log

# Check results
grep -E "(PASS|FAIL)" /tmp/contract-tests.log
```

Expected: All tests PASS

---

## Phase 6: Full Test Suite

### 6.1 Run Everything

```bash
# Complete test suite
go test ./... -v 2>&1 | tee /tmp/full-tests.log

# Summary
echo "=== Test Summary ==="
grep "^ok" /tmp/full-tests.log
grep "^FAIL" /tmp/full-tests.log || echo "No failures"
```

Expected: All packages show "ok", no FAIL lines

---

## Expected Outputs Summary

| Phase | Command | Expected Output |
|-------|---------|-----------------|
| Environment | `docker compose ps` | postgres and redis healthy |
| Build | `go build` | Binary created, no errors |
| Unit Tests | `go test ./internal/...` | All PASS |
| Lint | `make lint` | Exit code 0 |
| Health | `curl /health` | `{"status":"ok"}` |
| Auth URL | `curl /codex-auth-url` | URL + state JSON |
| Device Code | `curl /codex-device-code` | All device fields |
| Callback | `curl /callback` (invalid) | 404 + error JSON |
| Status | `curl /status` | `{"status":"wait"}` |
| Security | No management key | 401/403 |
| Rate Limit | Rapid requests | 429 after limit |
| Integration | `go test ./tests/...` | All PASS |

---

## Common Issues and Solutions

### Issue 1: Database Connection Failed

**Symptom**:
```
dial tcp 127.0.0.1:5432: connect: connection refused
```

**Solution**:
```bash
# Check if postgres is running
docker compose ps

# If not running, start it
docker compose up -d postgres

# Verify connection
docker compose exec postgres pg_isready -U postgres
```

### Issue 2: Redis Connection Failed

**Symptom**:
```
dial tcp 127.0.0.1:6379: connect: connection refused
```

**Solution**:
```bash
# Start redis
docker compose up -d redis

# Verify
docker compose exec redis redis-cli ping
```

### Issue 3: Missing Encryption Key

**Symptom**:
```
panic: ENCRYPTION_KEY environment variable is required
```

**Solution**:
```bash
export LLMPOOL_ENCRYPTION_KEY="your-32-byte-encryption-key-here"
# Or in .env file
echo "LLMPOOL_ENCRYPTION_KEY=your-key" > .env
```

### Issue 4: Management Key Rejected

**Symptom**:
```
HTTP Status: 401
{"status":"error","error":"invalid management key"}
```

**Solution**:
```bash
# Check current key
export LLMPOOL_MANAGEMENT_KEY="test-management-key"

# Use matching key in request
curl -H "X-Management-Key: test-management-key" ...
```

### Issue 5: Rate Limit Too Aggressive

**Symptom**:
```
HTTP Status: 429
{"status":"error","error":"rate limit exceeded"}
```

**Solution**:
```bash
# Wait for rate limit window to reset
sleep 60

# Or adjust rate limits in config
cat configs/default.yml | grep -A5 rate_limit
```

### Issue 6: Port Already in Use

**Symptom**:
```
bind: address already in use 0.0.0.0:8080
```

**Solution**:
```bash
# Find process using port
lsof -i :8080

# Kill it or use different port
export LLMPOOL_SERVER_PORT=8081
```

### Issue 7: Test Timeouts

**Symptom**:
```
timeout waiting for process to exit
```

**Solution**:
```bash
# Increase test timeout
go test ./... -timeout 5m

# Run specific package
go test ./internal/delivery/http/handler -v
```

### Issue 8: Lint Failures

**Symptom**:
```
internal/xxx/xxx.go:10:6: exported type Xxx should have comment
```

**Solution**:
```bash
# Auto-fix some issues
golangci-lint run --fix

# Or manually add comments, fix issues
```

---

## Quick Verification Script

Save this as `verify-oauth.sh`:

```bash
#!/bin/bash
set -e

echo "=== OAuth PKCE Verification ==="
echo

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

check() {
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓${NC} $1"
    else
        echo -e "${RED}✗${NC} $1"
        exit 1
    fi
}

# 1. Dependencies
echo "1. Checking dependencies..."
docker compose ps | grep -q "Up" && check "Services running"

# 2. Build
echo "2. Building application..."
go build -o /tmp/api ./cmd/api && check "Build successful"

# 3. Unit Tests
echo "3. Running unit tests..."
go test ./internal/... > /dev/null 2>&1 && check "Unit tests pass"

# 4. Lint
echo "4. Running linter..."
make lint > /dev/null 2>&1 && check "Lint passes"

# 5. Health Check
echo "5. Checking health endpoint..."
curl -s http://localhost:8080/health | grep -q "ok" && check "Health check passes"

# 6. Auth URL
echo "6. Testing auth URL endpoint..."
curl -s -H "X-Management-Key: ${LLMPOOL_MANAGEMENT_KEY:-test}" \
  http://localhost:8080/v1/internal/oauth/codex-auth-url | \
  grep -q "status.*ok" && check "Auth URL works"

echo
echo -e "${GREEN}=== All checks passed! ===${NC}"
```

Run it:
```bash
chmod +x verify-oauth.sh
./verify-oauth.sh
```

---

## Final Verification Checklist

Before considering implementation complete:

- [ ] All unit tests pass (`go test ./internal/...`)
- [ ] All integration tests pass (`go test ./tests/integration/...`)
- [ ] All regression tests pass (`go test ./tests/regression/...`)
- [ ] All contract tests pass (`go test ./tests/contract/...`)
- [ ] Lint passes (`make lint`)
- [ ] Application builds successfully
- [ ] Health endpoint returns `{"status":"ok"}`
- [ ] Auth URL endpoint returns valid state + PKCE
- [ ] Device code endpoint returns all required fields
- [ ] Callback validates state correctly
- [ ] Status endpoint returns wait/ok/error correctly
- [ ] Missing management key returns 401/403
- [ ] Rate limiting triggers after threshold
- [ ] State validation rejects invalid inputs
- [ ] Tokens are never logged in plain text
- [ ] Credentials stored encrypted in database

---

## Next Steps

After verification:

1. **Review Evidence**: Check all evidence files in `.sisyphus/learning/evidence/`
2. **Update Documentation**: Update README with OAuth endpoints
3. **Deploy**: Deploy to staging environment
4. **Monitor**: Set up monitoring for OAuth metrics
5. **Document**: Create user-facing OAuth setup guide

---

## Support

If issues persist after trying solutions above:

1. Check logs: `docker compose logs app`
2. Review session evidence files
3. Compare with working implementation in `.ref/CLIProxyAPI/`
4. Run individual test with verbose output: `go test -v -run TestName ./package`
