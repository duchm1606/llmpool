# Evidence Template

## Naming Convention

### Test Output Files
```
task-{N}-{scenario}.txt
```

Examples:
- `task-2-oauth-env-override.txt`
- `task-4-pkce-happy.txt`
- `task-8-code-exchange-failure.txt`

### Session Output Files
```
session-{N}-{slug}.md
```

Examples:
- `session-1-config-redis.md`
- `session-3-contracts-client.md`
- `session-5-callback-persistence.md`

### API Response Captures
```
task-{N}-{endpoint}.json
```

Examples:
- `task-10-native-auth-url.json`
- `task-11-callback-status-happy.json`
- `task-17-device-init.json`

---

## Evidence File Template

### Test Output Template

**Filename**: `.sisyphus/evidence/task-{N}-{scenario}.txt`

```
================================================================================
TASK {N} EVIDENCE: {Scenario Name}
================================================================================
Timestamp: {ISO 8601 timestamp}
Task: {Task description}
Scenario: {Scenario being tested}

--------------------------------------------------------------------------------
VERIFICATION COMMAND
--------------------------------------------------------------------------------
$ {exact command run}

--------------------------------------------------------------------------------
OUTPUT
--------------------------------------------------------------------------------
{command output}

--------------------------------------------------------------------------------
RESULT
--------------------------------------------------------------------------------
Status: [PASS / FAIL]
Exit Code: {exit code}

Expected: {expected result}
Actual: {actual result}
Match: [YES / NO]

--------------------------------------------------------------------------------
VERIFICATION CHECKLIST
--------------------------------------------------------------------------------
- [ ] Test executed successfully
- [ ] Output matches expected pattern
- [ ] No errors in output
- [ ] Deterministic (re-run produces same result)

--------------------------------------------------------------------------------
NOTES
--------------------------------------------------------------------------------
{Any observations, issues, or notes}

================================================================================
END OF EVIDENCE
================================================================================
```

---

### API Response Template

**Filename**: `.sisyphus/evidence/task-{N}-{endpoint}.json`

```json
{
  "metadata": {
    "task": "{N}",
    "scenario": "{scenario name}",
    "endpoint": "{endpoint path}",
    "timestamp": "{ISO 8601}",
    "request_id": "{optional}"
  },
  "request": {
    "method": "GET|POST|PUT|DELETE",
    "url": "{full url}",
    "headers": {
      "Content-Type": "application/json"
    },
    "body": {}
  },
  "response": {
    "status_code": 200,
    "headers": {},
    "body": {}
  },
  "validation": {
    "expected_fields": ["field1", "field2"],
    "actual_fields": ["field1", "field2"],
    "match": true,
    "notes": "{validation notes}"
  }
}
```

---

### Session Summary Template

**Filename**: `.sisyphus/evidence/session-{N}-{slug}.md`

```markdown
# Session {N} Evidence Summary

**Date**: {YYYY-MM-DD}
**Duration**: {actual duration} minutes
**Status**: [COMPLETE / PARTIAL / INCOMPLETE]

## Deliverables Checklist

- [ ] Code files created/modified
- [ ] Tests passing
- [ ] Build successful
- [ ] Evidence artifacts saved

## Files Modified

| File | Lines (+/-) | Status |
|------|-------------|--------|
| `path/to/file.go` | +{N} | Created |
| `path/to/file_test.go` | +{M} | Created |

## Test Results

```
{test output summary}
```

## Coverage

```
{coverage report summary}
```

## Issues Encountered

1. {Issue description}
   - Resolution: {how it was resolved}

## Notes for Next Session

{Handoff notes}

## Sign-off

- [ ] I verify that all deliverables are complete
- [ ] Evidence artifacts are saved in `.sisyphus/evidence/`
- [ ] Tests are deterministic and pass consistently

**Learner**: ___________
**Date**: ___________
```

---

## Evidence Directory Structure

```
.sisyphus/
├── evidence/
│   ├── task-1-template-check.txt
│   ├── task-1-evidence-convention.txt
│   ├── task-2-oauth-env-override.txt
│   ├── task-2-oauth-config-invalid.txt
│   ├── task-3-redis-bootstrap-happy.txt
│   ├── task-3-redis-bootstrap-failure.txt
│   ├── task-4-pkce-happy.txt
│   ├── task-4-pkce-negative.txt
│   ├── task-5-state-happy.txt
│   ├── task-5-state-negative.txt
│   ├── task-6-contract-compile.txt
│   ├── task-6-dependency-direction.txt
│   ├── task-7-contract-map-complete.txt
│   ├── task-7-contract-map-deviations.txt
│   ├── task-8-auth-url-build.txt
│   ├── task-8-code-exchange-failure.txt
│   ├── task-9-single-consume.txt
│   ├── task-9-ttl-expiry.txt
│   ├── task-10-native-auth-url.json
│   ├── task-10-compat-auth-url.json
│   ├── task-11-callback-status-happy.json
│   ├── task-11-callback-replay.json
│   ├── task-12-upsert-idempotent.txt
│   ├── task-12-sqlc-integrity.txt
│   ├── task-13-completion-persist.txt
│   ├── task-13-reauth-update.txt
│   ├── session-1-evidence.md
│   ├── session-2-evidence.md
│   ├── session-3-evidence.md
│   └── session-4-evidence.md
└── templates/
    ├── session-template.md
    └── evidence-template.md
```

---

## Quick Reference: Generating Evidence

### Test Output
```bash
# Run tests and save output
go test ./internal/{package} -run Test{Scenario} -v > .sisyphus/evidence/task-{N}-{scenario}.txt 2>&1
```

### API Response
```bash
# Capture API response
curl -s -X GET "http://localhost:8080/{endpoint}" \
  -H "Content-Type: application/json" \
  | jq '.' > .sisyphus/evidence/task-{N}-{endpoint}.json
```

### Coverage Report
```bash
# Generate coverage
go test ./internal/{package} -coverprofile=.sisyphus/evidence/coverage.out
go tool cover -func=.sisyphus/evidence/coverage.out > .sisyphus/evidence/task-{N}-coverage.txt
```

---

## Evidence Checklist

Before marking a task complete, ensure:

- [ ] Evidence file exists at correct path
- [ ] File follows naming convention
- [ ] Content matches template format
- [ ] Timestamp is included
- [ ] Result (PASS/FAIL) is clear
- [ ] Output is complete (not truncated)
- [ ] File is saved in `.sisyphus/evidence/`

---

**Template Version**: 1.0
**Last Updated**: {date}
