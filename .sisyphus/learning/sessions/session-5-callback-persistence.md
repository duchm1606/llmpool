# Session 5: Callback Handler + Encrypted Persistence

**Duration**: 60 minutes (20 concept / 35 coding / 5 recap)  
**Session File**: `session-5-callback-persistence.md`  
**Evidence Directory**: `evidence/session-5/`

---

## Objective

Implement the OAuth callback handler with encrypted credential persistence. This session completes the OAuth flow by handling authorization codes and securely storing tokens.

- [ ] Implement HandleCallback HTTP handler
- [ ] Integrate OAuth completion service for token exchange
- [ ] Implement encrypted credential upsert to database
- [ ] Validate state parameter against session store
- [ ] Handle error cases gracefully

---

## Prerequisites

Before starting this session, ensure:

- [ ] Previous session completed: Session 4 (Auth URL + Session Store)
- [ ] Reference files open:
  - `.sisyphus/docs/oauth-contract-map.md` - Section 2 (Callback Endpoint)
  - `internal/infra/oauth/state.go` - State validation utility
  - `internal/infra/oauth/session_store.go` - Session management
- [ ] Environment ready:
  - `docker compose up -d` running (postgres + redis)
  - `go test ./...` passes in current state
- [ ] Knowledge of:
  - OAuth callback flow mechanics
  - Encrypted credential storage patterns
  - PostgreSQL upsert operations

---

## Concept Review (20 min)

### Key Concepts

Brief explanation of concepts to understand before coding.

1. **Callback Flow**: After user authorizes at provider, browser redirects to callback with authorization code. Handler exchanges code for tokens.

2. **Token Exchange**: POST to token endpoint with code, client_id, client_secret (if applicable), redirect_uri, and code_verifier (PKCE).

3. **Encrypted Persistence**: Access tokens, refresh tokens, and metadata stored encrypted. Decryption only happens in-memory during use.

4. **State Validation**: Callback must validate state matches pending session to prevent CSRF attacks.

### Architecture Placement

Where this code fits in the Clean Architecture:

```
internal/
  delivery/
    http/
      handler/
        oauth_handler.go   <- HandleCallback (you are here)
  usecase/
    oauth/
      service.go           <- CompleteOAuthFlow (calls this)
  infra/
    credential/
      repository.go        <- UpsertCredential (persists here)
    oauth/
      client.go            <- ExchangeCodeForTokens (calls provider)
```

### Interface Contracts

```go
// Callback handler interface
type OAuthCallbackHandler interface {
    HandleCallback(c *gin.Context)
}

// Token exchange service interface
type OAuthCompletionService interface {
    CompleteOAuthFlow(ctx context.Context, req CompleteOAuthRequest) (*Credential, error)
}

// Encrypted credential repository interface
type CredentialRepository interface {
    Upsert(ctx context.Context, cred *Credential) error
    GetByProviderAndEmail(ctx context.Context, provider, email string) (*Credential, error)
}

// CompleteOAuthRequest - callback payload
type CompleteOAuthRequest struct {
    Provider    string `json:"provider" binding:"required"`
    Code        string `json:"code"`
    State       string `json:"state" binding:"required"`
    Error       string `json:"error"`
    RedirectURL string `json:"redirect_url"`
}
```

### Response Codes Reference

| Status | Body | Description |
|--------|------|-------------|
| 200 | `{"status":"ok"}` | Callback accepted, processing complete |
| 400 | `{"status":"error","error":"..."}` | Invalid request (missing state, invalid provider) |
| 404 | `{"status":"error","error":"unknown or expired state"}` | State not found or expired |
| 409 | `{"status":"error","error":"oauth flow is not pending"}` | Session already completed |
| 500 | `{"status":"error","error":"..."}` | Internal error during processing |

---

## RED Phase: Write Failing Tests (10 min)

### Test Cases to Write

- [ ] `TestHandleCallback_ValidCode` - Valid callback with authorization code succeeds
- [ ] `TestHandleCallback_InvalidState` - Unknown/expired state returns 404
- [ ] `TestHandleCallback_MismatchedProvider` - Provider doesn't match session returns 400
- [ ] `TestHandleCallback_ProviderError` - OAuth provider error in callback returns error status
- [ ] `TestHandleCallback_AlreadyCompleted` - Second callback for same state returns 409
- [ ] `TestHandleCallback_InvalidRedirectURL` - Malformed redirect_url parameter returns 400

### Test File Location

`internal/delivery/http/handler/oauth_callback_test.go`

### Expected Test Output (Save as Evidence)

Run tests to confirm they fail as expected:

```bash
cd /Users/duchoang/Projects/llmpool-worktrees/codex-pkce-learning-sessions
go test -v ./internal/delivery/http/handler -run "TestHandleCallback" 2>&1 | tee .sisyphus/learning/evidence/session-5-red.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-5-red.txt`

---

## GREEN Phase: Minimum Implementation (15 min)

### Implementation Steps

- [ ] Create `oauth_callback.go` with HandleCallback method
- [ ] Parse and validate callback request body
- [ ] Validate state parameter using existing ValidateState() utility
- [ ] Lookup pending session from session store
- [ ] Normalize provider name ("codex" and "openai" both map to "codex")
- [ ] Handle redirect_url parsing if provided (extract state, code, error from query params)
- [ ] Call OAuthCompletionService.CompleteOAuthFlow()
- [ ] Return appropriate HTTP responses per contract

### Implementation Location

`internal/delivery/http/handler/oauth_callback.go`

### OAuth Completion Service Steps

Inside `internal/usecase/oauth/service.go`:

- [ ] Exchange authorization code for tokens (POST to token endpoint)
- [ ] Include code_verifier from session (PKCE)
- [ ] Parse token response (access_token, refresh_token, expires_in, scope)
- [ ] Fetch user info to get email identifier
- [ ] Encrypt tokens using credential encryption
- [ ] Upsert to credential repository
- [ ] Mark session as completed

### Verification Commands

```bash
# Run specific tests
go test -v ./internal/delivery/http/handler -run "TestHandleCallback"

# Run all package tests
go test ./internal/delivery/http/handler

# Save passing output as evidence
go test -v ./internal/delivery/http/handler 2>&1 | tee .sisyphus/learning/evidence/session-5-green.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-5-green.txt`

---

## REFACTOR Phase: Clean Up (10 min)

### Refactoring Checklist

- [ ] Extract redirect URL parsing to helper function
- [ ] Add structured logging for each step (state validation, token exchange, persistence)
- [ ] Ensure auth codes never appear in logs (security)
- [ ] Add metrics: callback_count, callback_duration, callback_errors
- [ ] Extract provider normalization to shared utility
- [ ] Add request ID to context for tracing

### Refactoring Evidence

After refactoring, verify tests still pass:

```bash
go test ./internal/delivery/http/handler 2>&1 | tee .sisyphus/learning/evidence/session-5-refactor.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-5-refactor.txt`

---

## Recap (5 min)

### What Was Built

Summary of the implementation completed.

1. **HandleCallback HTTP Handler** - Receives OAuth callbacks, validates state, triggers token exchange
2. **OAuthCompletionService** - Orchestrates token exchange and credential persistence
3. **Encrypted Credential Upsert** - Stores tokens encrypted at rest
4. **Error Handling** - Proper HTTP status codes per OAuth contract

### Key Decisions

- Decision 1: Handler delegates all business logic to usecase service (clean architecture)
- Decision 2: Token exchange happens synchronously in callback handler (simpler than async)
- Decision 3: Credentials upsert by provider + email (handles re-authentication)

### Test Coverage

| Test | Status | Evidence |
|------|--------|----------|
| `TestHandleCallback_ValidCode` | PASS/FAIL | `session-5-green.txt` |
| `TestHandleCallback_InvalidState` | PASS/FAIL | `session-5-green.txt` |
| `TestHandleCallback_MismatchedProvider` | PASS/FAIL | `session-5-green.txt` |
| `TestHandleCallback_ProviderError` | PASS/FAIL | `session-5-green.txt` |
| `TestHandleCallback_AlreadyCompleted` | PASS/FAIL | `session-5-green.txt` |

### Links to Evidence

- RED: `.sisyphus/learning/evidence/session-5-red.txt`
- GREEN: `.sisyphus/learning/evidence/session-5-green.txt`
- REFACTOR: `.sisyphus/learning/evidence/session-5-refactor.txt`
- API Response: `.sisyphus/learning/evidence/session-5-response.json` (if applicable)

### Next Session Preview

Session 6: Refresh Contract + Worker - Implement token refresh mechanism and background worker

---

## Verify (Mandatory)

Before marking this session complete, run all verification commands:

### Unit Tests

```bash
# Run all tests in modified packages
go test -v ./internal/delivery/http/handler
go test -v ./internal/usecase/oauth
go test -v ./internal/infra/credential

# Run specific tests
go test -v ./internal/delivery/http/handler -run "TestHandleCallback"
```

### Integration Tests (if applicable)

```bash
# Start services if needed
docker compose up -d

# Test callback endpoint
curl -X POST http://localhost:8080/v1/internal/oauth/callback \
  -H "Content-Type: application/json" \
  -d '{"provider":"codex","code":"test","state":"test"}' | jq
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
go test ./... 2>&1 | tee .sisyphus/learning/evidence/session-5-verify.txt
```

**Evidence File**: `.sisyphus/learning/evidence/session-5-verify.txt`

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
