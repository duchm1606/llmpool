# Session 6: Refresh Contract + Worker

**Duration**: 60 minutes (20 concept / 35 coding / 5 recap)  
**Session File**: `session-6-refresh-worker.md`  
**Evidence Directory**: `evidence/session-6/`

---

## Objective

Implement the token refresh mechanism with background worker support. This session ensures credentials remain valid by automatically refreshing tokens before they expire.

- [ ] Define RefreshResult contract for token refresh operations
- [ ] Implement CodexRefresher with token refresh logic
- [ ] Create background worker for scheduled refresh
- [ ] Handle rotating refresh tokens properly
- [ ] Add metrics and observability for refresh operations

---

## Prerequisites

Before starting this session, ensure:

- [ ] Previous session completed: Session 5 (Callback Handler + Encrypted Persistence)
- [ ] Reference files open:
  - `.sisyphus/docs/oauth-contract-map.md` - Section 7.5 (Refresh Token Handling)
  - `internal/infra/credential/repository.go` - Credential storage
  - `internal/infra/config/config.go` - OAuth configuration
- [ ] Environment ready:
  - `docker compose up -d` running (postgres + redis)
  - `go test ./...` passes in current state
- [ ] Knowledge of:
  - OAuth refresh token flow
  - Background worker patterns in Go
  - Token rotation strategies

---

## Concept Review (20 min)

### Key Concepts

Brief explanation of concepts to understand before coding.

1. **Refresh Token Flow**: When access token expires, use refresh token to get new access token without user interaction. Response may include new refresh token (rotation).

2. **Rotating Refresh Tokens**: Security feature where each refresh yields a new refresh token. Old token invalidated. Must update storage atomically.

3. **Refresh Timing**: Refresh before expiration (e.g., at 80% of lifetime) to avoid service interruptions. Account for clock skew.

4. **Background Worker**: Goroutine that periodically checks for tokens nearing expiration and triggers refresh. Must handle concurrent refreshes safely.

### Architecture Placement

Where this code fits in the Clean Architecture:

```
internal/
  usecase/
    oauth/
      refresher.go       <- CodexRefresher interface (you are here)
  infra/
    oauth/
      codex_refresher.go <- CodexRefresher implementation
    refresh/
      worker.go          <- Background refresh worker
  domain/
    oauth/
      refresh_result.go  <- RefreshResult contract
```

### Interface Contracts

```go
// RefreshResult - outcome of token refresh operation
type RefreshResult struct {
    Success       bool          // Whether refresh succeeded
    AccessToken   string        // New access token (if success)
    RefreshToken  string        // New refresh token (may be same or rotated)
    ExpiresAt     time.Time     // New expiration time
    Scope         string        // Token scope
    Error         error         // Error if refresh failed
    Retryable     bool          // Whether error is retryable
}

// TokenRefresher interface for provider-specific refresh
type TokenRefresher interface {
    Refresh(ctx context.Context, refreshToken string) (*RefreshResult, error)
    Provider() string
}

// CodexRefresher implements TokenRefresher for Codex/OpenAI
type CodexRefresher struct {
    config     *config.OAuthConfig
    httpClient *http.Client
    logger     *zap.Logger
}

// RefreshWorker manages background token refresh
type RefreshWorker struct {
    refresher      TokenRefresher
    credRepo       CredentialRepository
    interval       time.Duration
    lookAhead      time.Duration // How far ahead to look for expiring tokens
    stopCh         chan struct{}
}
```

### Refresh Token Request/Response

**Request** (POST to token endpoint):
```
grant_type=refresh_token
refresh_token={refresh_token}
client_id={client_id}
```

**Response** (success):
```json
{
  "access_token": "new-access-token",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "new-refresh-token",  // May be same or different
  "scope": "read write"
}
```

---

## RED Phase: Write Failing Tests (10 min)

### Test Cases to Write

- [ ] `TestCodexRefresher_Success` - Valid refresh returns new tokens
- [ ] `TestCodexRefresher_TokenRotation` - Refresh with rotated tokens updates both
- [ ] `TestCodexRefresher_InvalidToken` - Invalid refresh token returns non-retryable error
- [ ] `TestCodexRefresher_NetworkError` - Network failure returns retryable error
- [ ] `TestRefreshWorker_StartStop` - Worker starts and stops cleanly
- [ ] `TestRefreshWorker_RefreshesExpiringTokens` - Worker finds and refreshes near-expiry tokens

### Test File Location

`internal/infra/oauth/codex_refresher_test.go`  
`internal/infra/refresh/worker_test.go`

### Expected Test Output (Save as Evidence)

Run tests to confirm they fail as expected:

```bash
cd /Users/duchoang/Projects/llmpool-worktrees/codex-pkce-learning-sessions
go test -v ./internal/infra/oauth -run "TestCodexRefresher" 2>&1 | tee .sisyphus/learning/evidence/session-6-red.txt
go test -v ./internal/infra/refresh -run "TestRefreshWorker" 2>&1 | tee -a .sisyphus/learning/evidence/session-6-red.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-6-red.txt`

---

## GREEN Phase: Minimum Implementation (15 min)

### Implementation Steps

#### 1. RefreshResult Domain Type

Create `internal/domain/oauth/refresh_result.go`:

```go
package oauth

import "time"

type RefreshResult struct {
    Success      bool
    AccessToken  string
    RefreshToken string
    ExpiresAt    time.Time
    Scope        string
    Error        error
    Retryable    bool
}

func (r *RefreshResult) IsSuccess() bool {
    return r.Success && r.Error == nil
}
```

#### 2. CodexRefresher Implementation

Create `internal/infra/oauth/codex_refresher.go`:

- [ ] Implement Refresh method
- [ ] POST to token URL with grant_type=refresh_token
- [ ] Parse response, handle rotating refresh tokens
- [ ] Return RefreshResult with proper error categorization
- [ ] Log at debug level (never log tokens)

#### 3. Refresh Worker Implementation

Create `internal/infra/refresh/worker.go`:

- [ ] Worker struct with configurable interval and look-ahead
- [ ] Start() method - begins ticker-based loop
- [ ] Stop() method - graceful shutdown via channel
- [ ] run() method - queries for expiring credentials, triggers refresh
- [ ] Prevent concurrent refresh of same credential (use Redis lock or DB flag)

### Implementation Locations

- `internal/domain/oauth/refresh_result.go` - Result contract
- `internal/infra/oauth/codex_refresher.go` - Codex implementation
- `internal/infra/refresh/worker.go` - Background worker

### Verification Commands

```bash
# Run specific tests
go test -v ./internal/infra/oauth -run "TestCodexRefresher"
go test -v ./internal/infra/refresh -run "TestRefreshWorker"

# Run all package tests
go test ./internal/infra/oauth ./internal/infra/refresh

# Save passing output as evidence
go test -v ./internal/infra/oauth ./internal/infra/refresh 2>&1 | tee .sisyphus/learning/evidence/session-6-green.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-6-green.txt`

---

## REFACTOR Phase: Clean Up (10 min)

### Refactoring Checklist

- [ ] Extract HTTP client configuration (timeout, retries)
- [ ] Add circuit breaker pattern for token endpoint failures
- [ ] Implement exponential backoff for retryable errors
- [ ] Add metrics: refresh_attempts_total, refresh_success_total, refresh_duration_seconds
- [ ] Create refresh queue for failed refreshes (retry later)
- [ ] Add structured logging with trace IDs
- [ ] Document why rotating refresh tokens require atomic updates

### Refactoring Evidence

After refactoring, verify tests still pass:

```bash
go test ./internal/infra/oauth ./internal/infra/refresh 2>&1 | tee .sisyphus/learning/evidence/session-6-refactor.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-6-refactor.txt`

---

## Recap (5 min)

### What Was Built

Summary of the implementation completed.

1. **RefreshResult Contract** - Standardized result type for all refresh operations
2. **CodexRefresher** - Provider-specific token refresh implementation
3. **RefreshWorker** - Background worker that proactively refreshes near-expiry tokens
4. **Token Rotation Support** - Proper handling of rotated refresh tokens

### Key Decisions

- Decision 1: Worker uses look-ahead window (e.g., 5 minutes) to refresh before expiration
- Decision 2: Concurrent refresh prevented via Redis distributed lock
- Decision 3: Errors categorized as retryable (network) vs non-retryable (invalid token)

### Test Coverage

| Test | Status | Evidence |
|------|--------|----------|
| `TestCodexRefresher_Success` | PASS/FAIL | `session-6-green.txt` |
| `TestCodexRefresher_TokenRotation` | PASS/FAIL | `session-6-green.txt` |
| `TestCodexRefresher_InvalidToken` | PASS/FAIL | `session-6-green.txt` |
| `TestRefreshWorker_StartStop` | PASS/FAIL | `session-6-green.txt` |
| `TestRefreshWorker_RefreshesExpiringTokens` | PASS/FAIL | `session-6-green.txt` |

### Links to Evidence

- RED: `.sisyphus/learning/evidence/session-6-red.txt`
- GREEN: `.sisyphus/learning/evidence/session-6-green.txt`
- REFACTOR: `.sisyphus/learning/evidence/session-6-refactor.txt`

### Next Session Preview

Session 7: Device Flow + Security - Implement device code flow and security guardrails

---

## Verify (Mandatory)

Before marking this session complete, run all verification commands:

### Unit Tests

```bash
# Run all tests in modified packages
go test -v ./internal/domain/oauth
go test -v ./internal/infra/oauth
go test -v ./internal/infra/refresh
```

### Integration Tests (if applicable)

```bash
# Start services if needed
docker compose up -d

# Test refresh logic manually (requires valid refresh token)
# This is typically tested via integration tests, not manually
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
go test ./... 2>&1 | tee .sisyphus/learning/evidence/session-6-verify.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-6-verify.txt`

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
