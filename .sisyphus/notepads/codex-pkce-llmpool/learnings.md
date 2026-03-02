# Learnings: OAuth Contract Mapping

## Date: 2026-03-02

### Contract Mapping Completed

Analyzed reference implementations from CLIProxyAPI and Proxypal to produce comprehensive OAuth contract map.

#### Key Findings from CLIProxyAPI:

1. **Auth URL Endpoint Pattern**
   - Base path: `/v0/management/{provider}-auth-url`
   - Providers: codex, anthropic, gemini-cli, qwen, iflow, antigravity, kimi
   - Query param: `is_webui=true` triggers callback forwarder mode
   - Response: `{status, url, state}`

2. **Callback Endpoint**
   - Path: `POST /v0/management/oauth-callback`
   - Body: `{provider, code, state, error, redirect_url}`
   - Provider normalization: "codex"/"openai" both map to "codex"
   - Returns: `{status: "ok"}` on success

3. **Status Polling Endpoint**
   - Path: `GET /v0/management/get-auth-status?state={state}`
   - Returns: `{status}` where status is "wait", "ok", or "error"
   - Session TTL: 10 minutes

4. **Session State Machine**
   - States: pending, completed, error, expired
   - Single-use semantics (replay returns 409)
   - Stored in memory with TTL

#### Key Findings from Proxypal:

1. **Field Compatibility Requirements**
   - Auth URL: expects `url`, `state` fields
   - Device code: expects `verification_uri` (or `verification_url` or `url`), `user_code`, `state` (or `device_code`), `expires_in`, `interval`
   - Status: checks `status == "ok"` for completion

2. **Header Requirements**
   - All management endpoints require: `X-Management-Key: {key}`

3. **Device Flow UX**
   - User sees verification_uri and user_code
   - Client polls with state/device_code
   - Completion detected when status becomes "ok"

#### llmpool Design Decisions Documented:

1. **Native paths**: `/v1/internal/oauth/...`
2. **Compatibility aliases**: `/v0/management/...` (Proxypal-compatible)
3. **Storage**: Encrypted DB instead of filesystem
4. **Sessions**: Redis-backed instead of in-memory
5. **Provider abstraction**: Interface-based for future extensibility

#### Deviations Section:
- Documented why llmpool differs from CLIProxyAPI
- Rationale for each deviation (security, scalability, clean architecture)

#### Document Location:
`/Users/duchoang/Projects/llmpool-worktrees/codex-pkce-learning-sessions/.sisyphus/docs/oauth-contract-map.md`
