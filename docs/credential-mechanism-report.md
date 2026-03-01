# Credential & Token Management Mechanism Report
**Project**: LLMPool  
**Date**: 2026-03-01  
**Scope**: Comparative analysis of credential discovery/import, refresh lifecycle, and security mechanisms from:
- `/.ref/proxypal`
- `/.ref/nango`

---

## 1) Executive Summary

Two strong reference patterns were analyzed:

- **proxypal** provides practical credential file lifecycle management (discover/upload/toggle/delete) with provider detection from filename conventions and provider-specific refresh writeback behavior.
- **nango** provides robust connection/token lifecycle architecture: refresh decision policies, lock + dedupe mechanisms, encryption-at-rest primitives, and provider-agnostic connector orchestration.

Current LLMPool implementation adopts a hybrid of both:
- file-based import + provider detection patterns inspired by proxypal
- refresh service + encryption scaffolding and provider abstraction inspired by nango

---

## 2) Evidence from proxypal (Mechanisms + Source Pointers)

## 2.1 Credential Discovery & File Lifecycle
Concrete implementation evidence:

- `/.ref/proxypal/src-tauri/src/commands/auth_files.rs`
  - `get_auth_files`
  - `upload_auth_file`
  - `delete_auth_file`
  - `toggle_auth_file`
  - `download_auth_file`
  - `delete_all_auth_files`

- `/.ref/proxypal/src/lib/tauri/auth-files.ts`
  - `getAuthFiles`
  - `uploadAuthFile`
  - `deleteAuthFile`
  - `toggleAuthFile`
  - `downloadAuthFile`
  - `deleteAllAuthFiles`

## 2.2 Provider Detection from Filename Conventions

- `/.ref/proxypal/src/pages/AuthFiles.tsx`
  - Auth file UX/provider inference logic using filename patterns

- `/.ref/proxypal/src-tauri/src/commands/auth.rs`
  - `refresh_auth_status` (provider-oriented auth status scanning)

- `/.ref/proxypal/src-tauri/src/commands/quota.rs`
  - `fetch_codex_quota` (codex-prefixed profile handling)
  - `fetch_antigravity_quota` (antigravity-prefixed profile handling)

## 2.3 Token Refresh + Writeback Pattern

- `/.ref/proxypal/src-tauri/src/commands/quota.rs`
  - `refresh_antigravity_token`
  - `fetch_antigravity_quota`

Observed mechanism:
- Check token expiry
- Use refresh token when available
- Persist updated token payload back to stored credential profile

---

## 3) Evidence from nango (Mechanisms + Source Pointers)

## 3.1 Refresh Decision Flow

- `/.ref/nango/packages/shared/lib/services/connections/credentials/refresh.ts`
  - `refreshOrTestCredentials`
  - `refreshCredentials`
  - `refreshCredentialsIfNeeded`
  - `shouldRefreshCredentials`

Observed mechanism:
- centralized refresh orchestration
- conditional refresh policy (expiry + heuristics)
- refresh failure state transitions

## 3.2 Locking + De-duplication

- `/.ref/nango/packages/kvstore/lib/Locking.ts`
  - `tryAcquire`
  - `acquire`
  - `release`
  - `withLock`

- `/.ref/nango/packages/shared/lib/services/connections/credentials/refresh.ts`
  - in-flight dedupe and lock-key-based refresh control

## 3.3 Encryption-at-Rest & Secret Handling

- `/.ref/nango/packages/shared/lib/utils/encryption.manager.ts`
  - `encryptConnection`
  - `decryptConnection`
  - `encryptProviderConfig`
  - `decryptProviderConfig`
  - `encryptAPISecret`
  - `encryptDatabaseIfNeeded`

## 3.4 Provider-Agnostic Connector Logic

- `/.ref/nango/packages/shared/lib/clients/provider.client.ts`
  - `shouldUseProviderClient`
  - `shouldIntrospectToken`
  - `getToken`
  - `refreshToken`
  - `introspectedTokenExpired`

- `/.ref/nango/packages/providers/lib/index.ts`
  - `getProvider`
  - `getProviders`
  - `loadProvidersYaml`

## 3.5 Scheduled Refresh Trigger

- `/.ref/nango/packages/server/lib/crons/refreshConnections.ts`
  - `exec`
  - `refreshConnectionsCron`

---

## 4) LLMPool Current Implementation Mapping

## 4.1 Implemented in LLMPool

### Provider-agnostic credential import
- `internal/usecase/credential/import_service.go`
  - supports filename + payload based provider detection
  - normalizes token field variants (`access_token`, `accessToken`, etc.)

### Internal import API endpoint
- `internal/delivery/http/handler/credential_handler.go`
  - `POST /v1/internal/auth-profiles/import`

### Domain/usecase structure
- `internal/domain/credential/entity.go`
- `internal/usecase/credential/interface.go`

### Refresh service scaffold
- `internal/usecase/credential/refresh_service.go`
- `internal/platform/server/refresh_worker.go`

### Secure token handling scaffold
- `internal/infra/security/encryptor.go`
  - AES-GCM based encrypt/decrypt utility
  - runtime encryption key required via env

### Required env enforcement
- `cmd/api/main.go`
  - hard requirement: `LLMPOOL_SECURITY_ENCRYPTION_KEY`
  - panic/fail-fast when missing

## 4.2 Verification tests currently present
- `internal/usecase/credential/import_service_test.go`
- `internal/infra/security/encryptor_test.go`
- `internal/infra/config/config_test.go`

---

## 5) Gap Analysis (What still differs from refs)

## 5.1 Storage maturity gap
Current:
- in-memory repository (`internal/infra/credential/memory_repository.go`)

Needed:
- persistent PostgreSQL repository with encrypted columns and migration strategy

## 5.2 Refresh maturity gap
Current:
- refresh scheduler exists
- provider refreshers are noop placeholders

Needed:
- real provider-specific refresh adapters
- refresh failure counters/cooldown windows

## 5.3 Concurrency/lock control gap
Current:
- no distributed locking around refresh cycles yet

Needed:
- Redis lock or DB advisory lock for multi-instance refresh safety (nango-style)
- in-flight dedupe semantics for same credential refresh

## 5.4 Operational lifecycle gap
Current:
- import + refresh scaffolding

Needed:
- profile status transitions and persistence model (`active`, `refresh_failed`, exhausted state)
- richer observability (refresh attempt success/failure metrics)

## 5.5 Credential lifecycle API parity gap
Current:
- import endpoint only

Needed:
- list/toggle/delete/download lifecycle endpoints (proxypal-style)

---

## 6) Recommended Implementation Sequence (Next)

1. Implement PostgreSQL credential repository (encrypted token fields + metadata columns)
2. Add provider refresher interfaces and first concrete refreshers (OpenAI/Codex, Anthropic)
3. Add refresh lock/dedupe strategy (Redis lock + singleflight-like in-process dedupe)
4. Add credential lifecycle endpoints (list/toggle/delete)
5. Add refresh failure counters, cooldown policy, and status transitions
6. Add telemetry: refresh success/failure/error-class metrics

---

## 7) Security Notes

- Encryption key is env-only by design (`LLMPOOL_SECURITY_ENCRYPTION_KEY`)
- Service fails fast if key is missing
- Avoid logging raw token values in handlers/usecases/loggers
- Replace in-memory store with encrypted DB persistence before production use

---

## 8) Conclusion

proxypal and nango provide complementary patterns:
- proxypal = practical credential file lifecycle + provider discovery behavior
- nango = production-grade refresh orchestration and secret lifecycle architecture

LLMPool now contains a validated base scaffold aligned to both references, with clear next steps to reach production-grade credential/token management.
