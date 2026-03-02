# Session {N}: {Session Title}

**Duration**: 60 minutes (20 concept / 35 coding / 5 recap)  
**Session File**: `session-{N}-{slug}.md`  
**Evidence Directory**: `evidence/session-{N}/`

---

## Objective

What you will accomplish in this session.

- [ ] Primary goal 1
- [ ] Primary goal 2
- [ ] Primary goal 3

---

## Prerequisites

Before starting this session, ensure:

- [ ] Previous session completed: Session {N-1}
- [ ] Reference files open:
  - {relevant reference file 1}
  - {relevant reference file 2}
- [ ] Environment ready:
  - `docker compose up -d` running
  - `go test ./...` passes in current state
- [ ] Knowledge of:
  - {concept 1}
  - {concept 2}

---

## Concept Review (20 min)

### Key Concepts

Brief explanation of concepts to understand before coding.

1. **Concept 1**: Description
2. **Concept 2**: Description
3. **Concept 3**: Description

### Architecture Placement

Where this code fits in the Clean Architecture:

```
internal/
  {layer}/
    {component}/
      {file}.go   <- You are here
```

### Interface Contracts

```go
// Define the interface(s) to implement
type {InterfaceName} interface {
    Method1() error
    Method2() (Result, error)
}
```

---

## RED Phase: Write Failing Tests (10 min)

### Test Cases to Write

- [ ] `{TestName1}` - Description of what this test validates
- [ ] `{TestName2}` - Description of what this test validates
- [ ] `{TestName3}` - Description of what this test validates

### Test File Location

`{path/to/test_file}_test.go`

### Expected Test Output (Save as Evidence)

Run tests to confirm they fail as expected:

```bash
cd /Users/duchoang/Projects/llmpool-worktrees/codex-pkce-learning-sessions
go test -v ./path/to/package -run "{TestPattern}" 2>&1 | tee .sisyphus/learning/evidence/task-{N}-red.txt
```

**Evidence File**: `.sisyphus/learning/evidence/task-{N}-red.txt`

---

## GREEN Phase: Minimum Implementation (15 min)

### Implementation Steps

- [ ] Create `{file}.go` with minimal implementation
- [ ] Implement method 1
- [ ] Implement method 2
- [ ] Ensure tests pass

### Implementation Location

`{path/to/implementation}.go`

### Verification Commands

```bash
# Run specific tests
go test -v ./path/to/package -run "{TestPattern}"

# Run all package tests
go test ./path/to/package

# Save passing output as evidence
go test -v ./path/to/package 2>&1 | tee .sisyphus/learning/evidence/task-{N}-green.txt
```

**Evidence File**: `.sisyphus/learning/evidence/task-{N}-green.txt`

---

## REFACTOR Phase: Clean Up (10 min)

### Refactoring Checklist

- [ ] Remove duplication
- [ ] Improve naming
- [ ] Add edge case handling
- [ ] Extract helper functions
- [ ] Improve error messages
- [ ] Add comments for non-obvious logic

### Refactoring Evidence

After refactoring, verify tests still pass:

```bash
go test ./path/to/package 2>&1 | tee .sisyphus/learning/evidence/task-{N}-refactor.txt
```

**Evidence File**: `.sisyphus/learning/evidence/task-{N}-refactor.txt`

---

## Recap (5 min)

### What Was Built

Summary of the implementation completed.

### Key Decisions

- Decision 1: Rationale
- Decision 2: Rationale

### Test Coverage

| Test | Status | Evidence |
|------|--------|----------|
| `{TestName1}` | PASS/FAIL | `task-{N}-{scenario}.txt` |
| `{TestName2}` | PASS/FAIL | `task-{N}-{scenario}.txt` |

### Links to Evidence

- RED: `.sisyphus/learning/evidence/task-{N}-red.txt`
- GREEN: `.sisyphus/learning/evidence/task-{N}-green.txt`
- REFACTOR: `.sisyphus/learning/evidence/task-{N}-refactor.txt`
- API Response: `.sisyphus/learning/evidence/task-{N}-response.json` (if applicable)

### Next Session Preview

Session {N+1}: {Next Session Title}

---

## Verify (Mandatory)

Before marking this session complete, run all verification commands:

### Unit Tests

```bash
# Run all tests in modified packages
go test -v ./internal/{layer}/{package}

# Run specific tests
go test -v ./internal/{layer}/{package} -run "{TestPrefix}"
```

### Integration Tests (if applicable)

```bash
# Start services if needed
docker compose up -d

# Run integration test
curl -s "http://localhost:8080/{endpoint}" | jq
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
go test ./... 2>&1 | tee .sisyphus/learning/evidence/task-{N}-verify.txt
```

**Evidence File**: `.sisyphus/learning/evidence/task-{N}-verify.txt`

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
