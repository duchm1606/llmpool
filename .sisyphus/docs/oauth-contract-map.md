# OAuth Contract Map

This document maps the OAuth endpoint contracts between CLIProxyAPI (reference implementation) and the llmpool native implementation, including compatibility aliases for Proxypal-style clients.

## Overview

CLIProxyAPI provides OAuth endpoints for multiple providers (Codex/OpenAI, Claude/Anthropic, Gemini, Qwen, iFlow, Antigravity, Kimi). llmpool implements Codex OAuth with CLIProxyAPI-compatible endpoint shapes while maintaining clean internal abstractions.

---

## 1. Auth URL Endpoint

### 1.1 Web OAuth Flow

**CLIProxyAPI Path:**
- `GET /v0/management/codex-auth-url`
- `GET /v0/management/anthropic-auth-url`
- `GET /v0/management/gemini-cli-auth-url`
- `GET /v0/management/qwen-auth-url`
- `GET /v0/management/iflow-auth-url`
- `GET /v0/management/antigravity-auth-url`
- `GET /v0/management/kimi-auth-url`

**llmpool Native:**
- `GET /v1/internal/oauth/codex-auth-url`

**llmpool Compatibility Alias:**
- `GET /v0/management/codex-auth-url`

**Request Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `is_webui` | bool | optional | When true, triggers callback forwarder mode for web UI clients |

**is_webui behavior:**
- CLIProxyAPI: Starts a callback forwarder on provider-specific port (codex: 1455, anthropic: 54545, gemini: 8085)
- llmpool: Compatibility alias preserves this parameter for Proxypal consumers

**Response Fields (200 OK):**

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | Always "ok" on success |
| `url` | string | Authorization URL with PKCE challenge and state |
| `state` | string | OAuth state parameter for CSRF protection |

**Example Response:**
```json
{
  "status": "ok",
  "url": "https://auth.openai.com/authorize?response_type=code&client_id=...&redirect_uri=...&code_challenge=...&code_challenge_method=S256&state=abc123",
  "state": "abc123"
}
```

**Error Responses:**
- 500: PKCE generation failure, state generation failure, callback server unavailable

---

### 1.2 Device Code Flow

**CLIProxyAPI Path:**
- `GET /v0/management/codex-auth-url` (without is_webui)
- `GET /v0/management/qwen-auth-url` (without is_webui)

**llmpool Native:**
- `GET /v1/internal/oauth/codex-device-code`

**llmpool Compatibility Alias:**
- `GET /v0/management/codex-auth-url` (no is_webui param)

**Request Parameters:**
- None specific for device flow

**Response Fields (200 OK):**

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | Always "ok" on success |
| `verification_uri` | string | URL for user to visit and enter code |
| `verification_url` | string | Alias for verification_uri (Proxypal compatibility) |
| `url` | string | Alias for verification_uri (Proxypal compatibility) |
| `user_code` | string | Code displayed to user |
| `state` | string | Device code/state for polling |
| `device_code` | string | Alias for state (Proxypal compatibility) |
| `expires_in` | number | Seconds until device code expires |
| `interval` | number | Seconds between poll requests |

**Example Response:**
```json
{
  "status": "ok",
  "verification_uri": "https://auth.openai.com/device",
  "user_code": "ABCD-EFGH",
  "state": "device-xyz789",
  "expires_in": 900,
  "interval": 5
}
```

**Status Semantics:**
- Provider returns device code immediately
- Client must poll for token completion
- User visits verification_uri and enters user_code

---

## 2. Callback Endpoint

### 2.1 Direct Callback (Web Flow)

**CLIProxyAPI Path:**
- Provider-specific paths like `/codex/callback`, `/anthropic/callback`
- Also supports file-based callback via `.oauth-{provider}-{state}.oauth` files

**llmpool Native:**
- `POST /v1/internal/oauth/callback`

**llmpool Compatibility Alias:**
- `POST /v0/management/oauth-callback`

**Request Body (JSON):**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `provider` | string | yes | Provider identifier ("codex", "openai") |
| `code` | string | conditional | Authorization code from OAuth provider |
| `state` | string | yes | OAuth state from auth URL response |
| `error` | string | conditional | Error message if OAuth failed |
| `redirect_url` | string | optional | Full redirect URL with query params (alternative input) |

**Provider Normalization:**
- "codex" and "openai" both normalize to "codex"
- "anthropic" and "claude" both normalize to "anthropic"
- "gemini" and "google" both normalize to "gemini"

**Redirect URL Parsing:**
If `redirect_url` is provided, the endpoint parses query parameters:
- `state` from `?state=...`
- `code` from `?code=...`
- `error` from `?error=...` or `?error_description=...`

**Response Fields:**

| Status | Body | Description |
|--------|------|-------------|
| 200 | `{"status":"ok"}` | Callback accepted, token exchange in progress |
| 400 | `{"status":"error","error":"..."}` | Invalid request (missing state, invalid provider) |
| 404 | `{"status":"error","error":"unknown or expired state"}` | State not found or expired |
| 409 | `{"status":"error","error":"oauth flow is not pending"}` | Session already completed or errored |
| 500 | `{"status":"error","error":"..."}` | Internal error during callback processing |

**Processing Flow:**
1. Validate state parameter (non-empty, valid characters, no path traversal)
2. Lookup pending session by state
3. Verify provider matches session
4. Write callback payload to trigger token exchange
5. Session transitions from "pending" to completion

---

## 3. Status Polling Endpoint

**CLIProxyAPI Path:**
- `GET /v0/management/get-auth-status?state={state}`

**llmpool Native:**
- `GET /v1/internal/oauth/status?state={state}`

**llmpool Compatibility Alias:**
- `GET /v0/management/get-auth-status?state={state}`

**Request Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `state` | string | yes | OAuth state from auth URL response |

**Response Status Semantics:**

| Status Value | Meaning | HTTP Status |
|--------------|---------|-------------|
| `wait` | Flow pending, user has not completed auth | 200 |
| `ok` | Flow completed successfully, credentials saved | 200 |
| `error` | Flow failed with error message | 200 (with error field) |

**Response Fields (200 OK):**

| Field | Type | Present When | Description |
|-------|------|--------------|-------------|
| `status` | string | always | One of: wait, ok, error |
| `error` | string | status=error | Error message for failed flows |

**Example Responses:**

Pending:
```json
{
  "status": "wait"
}
```

Success:
```json
{
  "status": "ok"
}
```

Error:
```json
{
  "status": "error",
  "error": "Failed to exchange authorization code for tokens"
}
```

**Proxypal Polling Behavior:**
- Polls every few seconds
- Checks `status == "ok"` to determine completion
- Returns `false` for non-success status codes or non-"ok" status

---

## 4. Device Status Polling Endpoint

**CLIProxyAPI Path:**
- Device flow uses same status endpoint as web flow
- Token polling happens internally in background goroutine

**llmpool Native:**
- `GET /v1/internal/oauth/device-status?state={state}`

**llmpool Compatibility Alias:**
- `GET /v0/management/get-auth-status?state={state}` (reuses web status)

**Request Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `state` | string | yes | Device code/state from device-code response |

**Response Status Semantics:**

| Status Value | Meaning |
|--------------|---------|
| `wait` | User has not yet authorized the device |
| `ok` | User authorized, tokens obtained and saved |
| `error` | Device code expired or authorization denied |

**Proxypal Device Flow:**
1. Call device-code endpoint (no is_webui)
2. Display `verification_uri` and `user_code` to user
3. Poll status endpoint with `state` (device_code)
4. When status becomes "ok", flow is complete

---

## 5. Session State Machine

**Session Lifecycle:**

```
[Register] -> PENDING -> [Callback] -> COMPLETED
                    |
                    -> [Error] -> ERROR
                    |
                    -> [TTL Expire] -> EXPIRED
```

**Session TTL:**
- CLIProxyAPI: 10 minutes (`oauthSessionTTL = 10 * time.Minute`)
- llmpool: Configurable, default 10 minutes

**State Semantics:**

| State | Description | Status Response |
|-------|-------------|-----------------|
| `pending` | Session created, waiting for callback | `wait` |
| `completed` | Callback received, tokens exchanged | `ok` |
| `error` | Error occurred during flow | `error` |
| `expired` | TTL elapsed without completion | (404 not found) |

**Single-Use Guarantee:**
- Sessions can only transition to completed once
- Replay attempts return 409 Conflict
- State is deleted after completion to prevent replay

---

## 6. Proxypal Compatibility Fields

Proxypal (Rust/Tauri client) has specific field expectations:

**Auth URL Response Compatibility:**
```rust
// Proxypal expects:
body["url"].as_str()   // Authorization URL
body["state"].as_str() // OAuth state
```

**Device Code Response Compatibility:**
```rust
// Proxypal accepts multiple field names:
body["verification_uri"]
  .or(body["verification_url"])
  .or(body["url"])

body["user_code"]

body["state"]
  .or(body["device_code"])

body["expires_in"]
body["interval"]
```

**Status Polling Compatibility:**
```rust
// Proxypal checks:
body["status"].as_str().unwrap_or("wait") == "ok"
```

**Management Key Header:**
- All management endpoints require: `X-Management-Key: {key}`

---

## 7. Deviations and Rationale

### 7.1 Storage Model

**CLIProxyAPI:**
- Stores credentials as JSON files on filesystem (`~/.cli-proxy-api/`)
- File naming: `{provider}-{email}.json` or `{provider}-{email}-{project}.json`

**llmpool:**
- Stores credentials encrypted in PostgreSQL
- DB is single source of truth, no filesystem persistence
- Filename patterns are informational only

**Rationale:**
- Encrypted DB persistence is a security requirement
- Enables horizontal scaling without shared filesystem
- Better for containerized deployments

### 7.2 Session Store

**CLIProxyAPI:**
- In-memory sync.Map with TTL
- Sessions lost on process restart

**llmpool:**
- Redis-backed session store with TTL
- Sessions survive process restart
- Enables horizontal scaling

**Rationale:**
- Redis provides durability and shared state across instances
- Required for high-availability deployments

### 7.3 Callback Mechanism

**CLIProxyAPI:**
- File-based signaling: writes `.oauth-{provider}-{state}.oauth` file
- Background goroutine polls for file
- Also supports HTTP callback paths

**llmpool:**
- HTTP POST callback endpoint
- Redis pub/sub or direct handler invocation
- No filesystem signaling

**Rationale:**
- Eliminates filesystem dependencies
- Cleaner for containerized environments
- HTTP callback is more standard

### 7.4 Provider Abstraction

**CLIProxyAPI:**
- Provider logic embedded in handlers
- Each provider has dedicated auth URL endpoint

**llmpool:**
- Provider-agnostic OAuth client interface
- Codex is only runtime-enabled provider
- Clean extension seam for Claude, Qwen, etc.

**Rationale:**
- Enables future providers without handler rewrites
- Cleaner architecture with dependency direction preserved

### 7.5 Refresh Token Handling

**CLIProxyAPI:**
- Stores refresh tokens in credential files
- Background refresh updates files

**llmpool:**
- Refresh tokens stored encrypted in DB
- Rotating refresh tokens properly handled
- Refresh worker updates encrypted payload

**Rationale:**
- Supports OAuth providers with rotating refresh tokens
- Security: tokens never in plaintext on filesystem

---

## 8. Endpoint Summary Table

| Operation | CLIProxyAPI Path | llmpool Native | llmpool Alias |
|-----------|------------------|----------------|---------------|
| Web Auth URL | `GET /v0/management/{provider}-auth-url` | `GET /v1/internal/oauth/codex-auth-url` | `GET /v0/management/codex-auth-url` |
| Device Code | `GET /v0/management/{provider}-auth-url` | `GET /v1/internal/oauth/codex-device-code` | `GET /v0/management/codex-auth-url` |
| Callback | `POST /v0/management/oauth-callback` | `POST /v1/internal/oauth/callback` | `POST /v0/management/oauth-callback` |
| Status Poll | `GET /v0/management/get-auth-status` | `GET /v1/internal/oauth/status` | `GET /v0/management/get-auth-status` |

---

## 9. Security Considerations

### 9.1 State Validation
- States must be alphanumeric with limited punctuation (`-`, `_`, `.`)
- No path separators (`/`, `\`)
- No traversal sequences (`..`)
- Maximum 128 characters

### 9.2 PKCE Requirements
- S256 challenge method required
- Verifier must be cryptographically random
- Verifier stored with session, never exposed in responses

### 9.3 Log Redaction
- Auth codes must never appear in logs
- Access tokens must never appear in logs
- Refresh tokens must never appear in logs
- State may be logged (not a secret)

### 9.4 Session Single-Use
- Sessions can only complete once
- Replays must be rejected with 409 Conflict
- Expired sessions return 404 (not distinguishable from invalid)

---

## 10. Test Coverage Requirements

Per the implementation plan, the following contract behaviors must be tested:

| Scenario | Test Type | Verification |
|----------|-----------|--------------|
| Auth URL returns valid state + PKCE | Unit | URL contains code_challenge, state returned |
| Device code returns all required fields | Unit | verification_uri, user_code, state, expires_in, interval |
| Status returns wait/ok/error | Integration | State transitions correctly |
| Callback validates state | Integration | Invalid state returns 400/404 |
| Callback validates provider | Integration | Mismatched provider returns 400 |
| Replay prevention | Integration | Second callback returns 409 |
| TTL expiry | Integration | Expired session returns 404 |
| Compatibility alias shape | Integration | Response matches Proxypal expectations |
| Log redaction | Security | No tokens in logs |

---

## References

- CLIProxyAPI handlers: `.ref/CLIProxyAPI/internal/api/handlers/management/`
  - `auth_files.go` - Auth URL endpoints
  - `oauth_callback.go` - Callback handler
  - `oauth_sessions.go` - Session lifecycle
- Proxypal client: `.ref/proxypal/src-tauri/src/commands/auth.rs`
- Implementation plan: `.sisyphus/plans/codex-pkce-llmpool.md`
