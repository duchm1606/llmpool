# Session 8: Integration + Capstone

**Duration**: 60 minutes (20 concept / 35 coding / 5 recap)  
**Session File**: `session-8-integration-capstone.md`  
**Evidence Directory**: `evidence/session-8/`

---

## Objective

Complete the OAuth implementation with full integration tests and regression suite. This session verifies the entire OAuth flow works end-to-end and documents everything for production readiness.

- [ ] Write integration tests for full OAuth web flow
- [ ] Write integration tests for device flow
- [ ] Create regression test suite
- [ ] Verify Proxypal compatibility
- [ ] Final security audit
- [ ] Documentation completion

---

## Prerequisites

Before starting this session, ensure:

- [ ] Previous sessions completed: Sessions 1-7
- [ ] Reference files open:
  - `.sisyphus/docs/oauth-contract-map.md` - Full contract reference
  - All previous session evidence files
  - `README.md` for documentation updates
- [ ] Environment ready:
  - `docker compose up -d` running (postgres + redis)
  - `go test ./...` passes in current state
  - Mock OAuth server or test credentials available
- [ ] Knowledge of:
  - Integration testing patterns
  - Testcontainers or Docker-based testing
  - OAuth end-to-end flows

---

## Concept Review (20 min)

### Key Concepts

Brief explanation of concepts to understand before coding.

1. **Integration Testing**: Tests that verify multiple components work together. Uses real (or realistic) dependencies, not mocks.

2. **OAuth E2E Flow**: Complete flow from auth URL -> user authorization (simulated) -> callback -> token storage -> API usage.

3. **Regression Suite**: Automated tests that catch regressions. Run before every release. Covers critical paths.

4. **Contract Testing**: Verify external consumers (Proxypal) can use our API. Match field names, types, and behaviors.

### Architecture Placement

Where this code fits in the Clean Architecture:

```
tests/
  integration/
    oauth_flow_test.go      <- Full OAuth flow tests (you are here)
    device_flow_test.go     <- Device flow integration tests
  regression/
    oauth_regression_test.go <- Regression suite
  contract/
    proxypal_compat_test.go  <- Proxypal compatibility tests
```

### Test Categories

| Category | Scope | Dependencies | Speed |
|----------|-------|--------------|-------|
| Unit | Single function | Mocks only | Fast |
| Integration | Multiple components | Real DB, Redis | Medium |
| E2E | Full system | All services | Slow |
| Contract | External API shape | HTTP only | Medium |

### Integration Test Scenarios

```go
// Web Flow Integration Test
func TestOAuthWebFlow_EndToEnd(t *testing.T) {
    // 1. Request auth URL
    // 2. Verify state and PKCE stored in session
    // 3. Simulate callback from provider
    // 4. Verify tokens exchanged and stored
    // 5. Verify credential encrypted in DB
}

// Device Flow Integration Test
func TestOAuthDeviceFlow_EndToEnd(t *testing.T) {
    // 1. Request device code
    // 2. Verify device session created
    // 3. Simulate user authorization
    // 4. Poll for completion
    // 5. Verify tokens stored
}
```

---

## RED Phase: Write Failing Tests (10 min)

### Test Cases to Write

#### Integration Tests

- [ ] `TestIntegration_WebFlow_Success` - Complete web flow happy path
- [ ] `TestIntegration_WebFlow_CallbackReplay` - Second callback returns 409
- [ ] `TestIntegration_WebFlow_StateExpiry` - Expired state returns 404
- [ ] `TestIntegration_DeviceFlow_Success` - Complete device flow happy path
- [ ] `TestIntegration_DeviceFlow_PollBeforeAuthorize` - Poll returns wait
- [ ] `TestIntegration_TokenRefresh` - Expired token gets refreshed

#### Regression Tests

- [ ] `TestRegression_AuthURLCompatibility` - Response matches Proxypal expectations
- [ ] `TestRegression_CallbackCompatibility` - Endpoint accepts Proxypal payload
- [ ] `TestRegression_StatusCompatibility` - Status response format correct
- [ ] `TestRegression_DeviceCodeCompatibility` - Device code response has all aliases

#### Contract Tests

- [ ] `TestContract_ProxypalAuthURL` - Fields: url, state
- [ ] `TestContract_ProxypalDeviceCode` - Fields: verification_uri, user_code, state, expires_in, interval
- [ ] `TestContract_ProxypalStatus` - Status values: wait, ok, error

### Test File Location

`tests/integration/oauth_flow_test.go`  
`tests/regression/oauth_regression_test.go`  
`tests/contract/proxypal_compat_test.go`

### Expected Test Output (Save as Evidence)

Run tests to confirm they fail as expected:

```bash
cd /Users/duchoang/Projects/llmpool-worktrees/codex-pkce-learning-sessions
go test -v ./tests/integration -run "TestIntegration" 2>&1 | tee .sisyphus/learning/evidence/session-8-red.txt
go test -v ./tests/regression -run "TestRegression" 2>&1 | tee -a .sisyphus/learning/evidence/session-8-red.txt
go test -v ./tests/contract -run "TestContract" 2>&1 | tee -a .sisyphus/learning/evidence/session-8-red.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-8-red.txt`

---

## GREEN Phase: Minimum Implementation (15 min)

### Implementation Steps

#### 1. Integration Test Setup

Create `tests/integration/oauth_flow_test.go`:

- [ ] Test fixtures for test database
- [ ] Test server setup with real dependencies
- [ ] Mock OAuth provider (or use httptest)
- [ ] Helper functions for common operations

#### 2. Mock OAuth Provider

Create `tests/integration/mock_oauth_server.go`:

- [ ] Mock authorization endpoint
- [ ] Mock token endpoint (authorization code exchange)
- [ ] Mock device code endpoint
- [ ] Mock device token endpoint
- [ ] Configurable responses for error scenarios

#### 3. Regression Tests

Create `tests/regression/oauth_regression_test.go`:

- [ ] Test each compatibility alias endpoint
- [ ] Verify response field names match Proxypal expectations
- [ ] Test error response formats
- [ ] Verify status code mappings

#### 4. Contract Tests

Create `tests/contract/proxypal_compat_test.go`:

- [ ] Test field presence in responses
- [ ] Test field types (string, int, bool)
- [ ] Test enum values (status: wait/ok/error)
- [ ] Document any intentional deviations

### Implementation Locations

- `tests/integration/oauth_flow_test.go` - Integration tests
- `tests/integration/mock_oauth_server.go` - Mock provider
- `tests/regression/oauth_regression_test.go` - Regression suite
- `tests/contract/proxypal_compat_test.go` - Contract tests

### Verification Commands

```bash
# Run specific tests
go test -v ./tests/integration -run "TestIntegration"
go test -v ./tests/regression -run "TestRegression"
go test -v ./tests/contract -run "TestContract"

# Run all test suites
go test -v ./tests/... 2>&1 | tee .sisyphus/learning/evidence/session-8-green.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-8-green.txt`

---

## REFACTOR Phase: Clean Up (10 min)

### Refactoring Checklist

- [ ] Extract test helpers to shared package
- [ ] Add test parallelization where safe
- [ ] Create test data builders for fixtures
- [ ] Add test cleanup (truncate tables, clear Redis)
- [ ] Document test setup requirements
- [ ] Create Makefile target for integration tests
- [ ] Add CI/CD pipeline configuration for test automation

### Refactoring Evidence

After refactoring, verify tests still pass:

```bash
go test ./tests/... 2>&1 | tee .sisyphus/learning/evidence/session-8-refactor.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-8-refactor.txt`

---

## Recap (5 min)

### What Was Built

Summary of the implementation completed.

1. **Integration Tests** - Full OAuth web flow and device flow E2E tests
2. **Mock OAuth Provider** - Configurable test double for provider responses
3. **Regression Suite** - Automated tests for critical paths
4. **Contract Tests** - Proxypal compatibility verification
5. **Documentation** - Test setup and running instructions

### Key Decisions

- Decision 1: Integration tests use Docker Compose dependencies (realistic)
- Decision 2: Mock OAuth provider enables offline testing
- Decision 3: Regression tests run on every PR (CI gate)
- Decision 4: Contract tests document external API commitments

### Test Coverage

| Test | Status | Evidence |
|------|--------|----------|
| `TestIntegration_WebFlow_Success` | PASS/FAIL | `session-8-green.txt` |
| `TestIntegration_DeviceFlow_Success` | PASS/FAIL | `session-8-green.txt` |
| `TestRegression_AuthURLCompatibility` | PASS/FAIL | `session-8-green.txt` |
| `TestContract_ProxypalAuthURL` | PASS/FAIL | `session-8-green.txt` |

### Links to Evidence

- RED: `.sisyphus/learning/evidence/session-8-red.txt`
- GREEN: `.sisyphus/learning/evidence/session-8-green.txt`
- REFACTOR: `.sisyphus/learning/evidence/session-8-refactor.txt`
- Integration Output: `.sisyphus/learning/evidence/session-8-integration.log`

### Final Deliverables

- [ ] All integration tests passing
- [ ] Capstone runbook complete
- [ ] Security audit passed
- [ ] Documentation updated
- [ ] Ready for production

---

## Verify (Mandatory)

Before marking this session complete, run all verification commands:

### Unit Tests

```bash
# Run all unit tests
go test ./internal/...
```

### Integration Tests

```bash
# Start services
docker compose up -d

# Run integration tests
go test -v ./tests/integration

# Run regression tests
go test -v ./tests/regression

# Run contract tests
go test -v ./tests/contract
```

### Full Test Suite

```bash
# All tests (unit + integration)
go test ./... 2>&1 | tee .sisyphus/learning/evidence/session-8-full-test.txt
```

### Lint

```bash
make lint 2>&1 | tee .sisyphus/learning/evidence/session-8-lint.txt
```

### Security Scan

```bash
# If gosec is available
gosec ./... 2>&1 | tee .sisyphus/learning/evidence/session-8-security.txt

# Check for secrets in code
git-secrets --scan 2>&1 | tee -a .sisyphus/learning/evidence/session-8-security.txt
```

### Evidence Collection

Save final verification output:

```bash
# Full verification
echo "=== Unit Tests ===" > .sisyphus/learning/evidence/session-8-verify.txt
go test ./internal/... >> .sisyphus/learning/evidence/session-8-verify.txt 2>&1

echo "" >> .sisyphus/learning/evidence/session-8-verify.txt
echo "=== Integration Tests ===" >> .sisyphus/learning/evidence/session-8-verify.txt
go test ./tests/... >> .sisyphus/learning/evidence/session-8-verify.txt 2>&1

echo "" >> .sisyphus/learning/evidence/session-8-verify.txt
echo "=== Lint ===" >> .sisyphus/learning/evidence/session-8-verify.txt
make lint >> .sisyphus/learning/evidence/session-8-verify.txt 2>&1
```

**Evidence File**: `.sisyphus/learning/evidence/session-8-verify.txt`

---

## Session Complete Checklist

- [ ] All RED tests written and failing
- [ ] GREEN phase: minimum implementation passes tests
- [ ] REFACTOR phase: code cleaned, tests still pass
- [ ] Evidence files saved in `.sisyphus/learning/evidence/`
- [ ] Full test suite passes: `go test ./...`
- [ ] Integration tests pass: `go test ./tests/...`
- [ ] Lint passes: `make lint`
- [ ] Security scan passes (if applicable)
- [ ] Capstone runbook reviewed
- [ ] All sessions complete

---

## Capstone Achievement

Congratulations! You have completed the full OAuth PKCE implementation for llmpool.

### What Was Built (All Sessions)

1. **Session 1-2**: OAuth config, PKCE utilities, state generation
2. **Session 3-4**: Auth URL endpoint, session store
3. **Session 5**: Callback handler, encrypted credential persistence
4. **Session 6**: Token refresh, background worker
5. **Session 7**: Device flow, security guardrails
6. **Session 8**: Integration tests, regression suite

### Production Readiness

- Comprehensive test coverage (unit + integration)
- Security audit passed
- Proxypal compatibility verified
- Documentation complete
- Ready for deployment
