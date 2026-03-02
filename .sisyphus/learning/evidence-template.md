# Evidence Collection Template

This template defines naming conventions and formats for capturing learning session evidence.

---

## Naming Conventions

### Session Files

**Format**: `session-{N}-{slug}.md`

| Session | Filename |
|---------|----------|
| Session 1 | `session-1-config-redis.md` |
| Session 2 | `session-2-pkce-state.md` |
| Session 3 | `session-3-contracts-client.md` |
| Session 4 | `session-4-session-store.md` |
| Session 5 | `session-5-callback-persistence.md` |
| Session 6 | `session-6-refresh.md` |
| Session 7 | `session-7-device-flow.md` |
| Session 8 | `session-8-regression.md` |

### Test Output Evidence

**Format**: `task-{N}-{scenario}.txt`

| Phase | Example Filename | Description |
|-------|------------------|-------------|
| RED | `task-1-red.txt` | Initial failing tests |
| GREEN | `task-1-green.txt` | Tests passing after implementation |
| REFACTOR | `task-1-refactor.txt` | Tests after refactoring |
| Verify | `task-1-verify.txt` | Final verification output |

**Scenario Variants**:
- `task-{N}-unit.txt` - Unit test output
- `task-{N}-integration.txt` - Integration test output
- `task-{N}-negative.txt` - Negative test cases
- `task-{N}-{specific-test}.txt` - Specific test output

### API Response Evidence

**Format**: `task-{N}-{scenario}.json`

| Scenario | Example Filename |
|----------|------------------|
| Auth URL | `task-3-auth-url-response.json` |
| Callback | `task-5-callback-response.json` |
| Status | `task-4-status-response.json` |
| Device Code | `task-7-device-code-response.json` |
| Device Status | `task-7-device-status-response.json` |

---

## Directory Structure

```
.sisyphus/learning/
├── session-template.md              <- Reusable session template
├── evidence-template.md             <- This file
├── evidence/
│   ├── task-1-red.txt
│   ├── task-1-green.txt
│   ├── task-1-refactor.txt
│   ├── task-1-verify.txt
│   ├── task-2-red.txt
│   ├── task-2-green.txt
│   ├── task-2-pkce-verifier.txt
│   ├── task-2-state-validation.txt
│   ├── task-3-red.txt
│   ├── task-3-green.txt
│   ├── task-3-auth-url-response.json
│   ├── task-4-red.txt
│   ├── task-4-green.txt
│   ├── task-4-session-ttl.txt
│   ├── task-5-red.txt
│   ├── task-5-green.txt
│   ├── task-5-callback-response.json
│   ├── task-6-red.txt
│   ├── task-6-green.txt
│   ├── task-6-refresh-rotation.txt
│   ├── task-7-red.txt
│   ├── task-7-green.txt
│   ├── task-7-device-code-response.json
│   ├── task-7-device-status-response.json
│   ├── task-8-integration.txt
│   └── task-8-verify.txt
```

---

## Evidence File Formats

### Test Output Format (.txt)

**Content**: Raw test output from `go test -v`

```
=== RUN   TestPKCEVerifier_Generate
=== PAUSE TestPKCEVerifier_Generate
=== CONT  TestPKCEVerifier_Generate
--- PASS: TestPKCEVerifier_Generate (0.00s)
    pkce_test.go:15: Generated verifier length: 128
=== RUN   TestPKCEVerifier_S256Challenge
=== PAUSE TestPKCEVerifier_S256Challenge
=== CONT  TestPKCEVerifier_S256Challenge
--- PASS: TestPKCEVerifier_S256Challenge (0.00s)
    pkce_test.go:30: Challenge: E9x4...x2L8
PASS
ok      github.com/your-org/llmpool/internal/usecase/oauth      0.123s
```

**Command to Generate**:
```bash
go test -v ./internal/usecase/oauth -run "TestPKCE" 2>&1 | tee .sisyphus/learning/evidence/task-2-pkce-verifier.txt
```

### API Response Format (.json)

**Content**: Pretty-printed JSON responses from curl

```json
{
  "status": "ok",
  "url": "https://auth.openai.com/authorize?response_type=code&...",
  "state": "a1b2c3d4e5f6..."
}
```

**Command to Generate**:
```bash
curl -s "http://localhost:8080/v1/internal/oauth/codex-auth-url" | jq . > .sisyphus/learning/evidence/task-3-auth-url-response.json
```

---

## Required Evidence Per Session

### All Sessions Must Include:

| Evidence | Format | Description |
|----------|--------|-------------|
| RED | `.txt` | Failing tests before implementation |
| GREEN | `.txt` | Passing tests after minimum implementation |
| REFACTOR | `.txt` | Tests still passing after cleanup |
| Verify | `.txt` | Full `go test ./...` output |

### Session-Specific Evidence:

**Session 1 (Config + Redis)**:
- `task-1-red.txt`, `task-1-green.txt`, `task-1-refactor.txt`, `task-1-verify.txt`

**Session 2 (PKCE + State)**:
- `task-2-red.txt`, `task-2-green.txt`, `task-2-refactor.txt`
- `task-2-pkce-verifier.txt` - PKCE verifier generation test
- `task-2-state-validation.txt` - State validation negative tests

**Session 3 (Contracts + Client)**:
- `task-3-red.txt`, `task-3-green.txt`, `task-3-refactor.txt`
- `task-3-auth-url-response.json` - Live auth URL response

**Session 4 (Session Store)**:
- `task-4-red.txt`, `task-4-green.txt`, `task-4-refactor.txt`
- `task-4-session-ttl.txt` - TTL expiry test
- `task-4-status-response.json` - Status endpoint response

**Session 5 (Callback + Persistence)**:
- `task-5-red.txt`, `task-5-green.txt`, `task-5-refactor.txt`
- `task-5-callback-response.json` - Callback response

**Session 6 (Refresh)**:
- `task-6-red.txt`, `task-6-green.txt`, `task-6-refactor.txt`
- `task-6-refresh-rotation.txt` - Refresh token rotation test

**Session 7 (Device Flow)**:
- `task-7-red.txt`, `task-7-green.txt`, `task-7-refactor.txt`
- `task-7-device-code-response.json` - Device code response
- `task-7-device-status-response.json` - Device status response

**Session 8 (Regression)**:
- `task-8-integration.txt` - End-to-end integration tests
- `task-8-verify.txt` - Full verification suite

---

## Verification Command Examples

### Unit Test Verification

```bash
# Single package
go test -v ./internal/infra/config 2>&1 | tee .sisyphus/learning/evidence/task-1-verify.txt

# With specific test pattern
go test -v ./internal/usecase/oauth -run "TestPKCE" 2>&1 | tee .sisyphus/learning/evidence/task-2-pkce-verifier.txt

# All tests
go test ./... 2>&1 | tee .sisyphus/learning/evidence/task-{N}-verify.txt
```

### Integration Verification

```bash
# Start dependencies
docker compose up -d

# Test auth URL endpoint
curl -s "http://localhost:8080/v1/internal/oauth/codex-auth-url" | jq . > .sisyphus/learning/evidence/task-3-auth-url-response.json

# Test compatibility alias
curl -s "http://localhost:8080/v0/management/codex-auth-url?is_webui=true" | jq . > .sisyphus/learning/evidence/task-3-compat-auth-url-response.json
```

### Lint Verification

```bash
make lint 2>&1 | tee .sisyphus/learning/evidence/task-{N}-lint.txt
```

---

## Evidence Collection Workflow

1. **Before coding**: Write tests (RED phase)
   ```bash
   go test -v ./path -run "TestPattern" 2>&1 | tee .sisyphus/learning/evidence/task-{N}-red.txt
   ```

2. **During implementation**: Save passing tests (GREEN phase)
   ```bash
   go test -v ./path 2>&1 | tee .sisyphus/learning/evidence/task-{N}-green.txt
   ```

3. **After refactoring**: Verify still passing (REFACTOR phase)
   ```bash
   go test -v ./path 2>&1 | tee .sisyphus/learning/evidence/task-{N}-refactor.txt
   ```

4. **Before session end**: Full verification
   ```bash
   go test ./... 2>&1 | tee .sisyphus/learning/evidence/task-{N}-verify.txt
   ```

---

## Evidence Checklist (Per Session)

- [ ] `task-{N}-red.txt` - Initial failing tests saved
- [ ] `task-{N}-green.txt` - Tests passing after implementation
- [ ] `task-{N}-refactor.txt` - Tests passing after refactoring
- [ ] `task-{N}-verify.txt` - Full verification output
- [ ] Session-specific `.txt` files (if applicable)
- [ ] Session-specific `.json` files (if applicable)
- [ ] Evidence files committed to git (optional)
