# Session Template: {Session N} - {Title}

## Metadata
- **Session Number**: {N}
- **Title**: {Short descriptive title}
- **Duration**: 60 minutes
- **Format**: 20 min concept + 35 min coding + 5 min recap
- **Prerequisites**: {List previous sessions or knowledge required}

---

## Learning Objectives

By the end of this session, you will:
1. {Objective 1}
2. {Objective 2}
3. {Objective 3}

---

## Prerequisites Check

Before starting, ensure you have:
- [ ] {Prerequisite 1}
- [ ] {Prerequisite 2}
- [ ] {Prerequisite 3}

---

## RED Phase (Write Failing Tests) - 20 minutes

### Concept: {Brief concept explanation}

### Tasks

1. **Create test file**: `{file}_test.go`
   ```bash
   touch internal/{package}/{file}_test.go
   ```

2. **Write test cases**:
   - [ ] Test {scenario 1}
   - [ ] Test {scenario 2}
   - [ ] Test {error case}

3. **Run tests** (should fail):
   ```bash
   go test ./internal/{package} -run Test{N} -v
   ```

### Expected Result
Tests should fail with: `{expected error message}`

---

## GREEN Phase (Make Tests Pass) - 15 minutes

### Implementation Tasks

1. **Create implementation file**: `{file}.go`

2. **Implement functions**:
   ```go
   func {FunctionName}(params) (return, error) {
       // Implementation here
   }
   ```

3. **Run tests** (should pass):
   ```bash
   go test ./internal/{package} -run Test{N} -v
   ```

### Verification
- [ ] All tests pass
- [ ] Build succeeds: `go build ./...`

---

## REFACTOR Phase (Clean Code) - 15 minutes

### Review Checklist

- [ ] Code follows project conventions
- [ ] Error messages are descriptive
- [ ] Edge cases are handled
- [ ] No hardcoded values
- [ ] Clean Architecture boundaries respected

### Refactoring Tasks

1. {Refactoring task 1}
2. {Refactoring task 2}
3. {Refactoring task 3}

---

## Recap & Checkpoint (5 minutes)

### Key Takeaways

1. {Key learning 1}
2. {Key learning 2}
3. {Key learning 3}

### Code Deliverables

Files created/modified:
- `internal/{package}/{file}.go`
- `internal/{package}/{file}_test.go`

### Verification Commands

Run these to verify completion:
```bash
# Run all tests for this session
go test ./internal/{package} -run Test{N} -v

# Run full test suite
go test ./...

# Build check
go build ./...
```

### Evidence Artifacts

Save these files:
- [ ] Test output: `.sisyphus/evidence/session-{N}-tests.txt`
- [ ] Coverage report: `.sisyphus/evidence/session-{N}-coverage.txt`

---

## Next Session Handoff

### For Session {N+1}:

- **Topic**: {Next session topic}
- **Depends on**: {What from this session is needed}
- **Setup required**: {Any setup needed}

### Blockers

- [ ] {Any blockers for next session}

---

## "If Stuck" Decision Points

### If tests don't compile:
1. Check imports are correct
2. Verify interface signatures match
3. Check for typos in function names

### If tests fail unexpectedly:
1. Read error message carefully
2. Check test setup (fixtures, mocks)
3. Verify expected vs actual values

### If behind on time:
- **20 min mark**: Skip to minimal implementation
- **35 min mark**: Skip refactoring, document tech debt
- **50 min mark**: Save state, continue in next session

---

## Resources

### Reference Materials
- {Link to relevant doc}
- {Link to external resource}

### Related Files
- `internal/{package}/...`
- `internal/{other}/...`

---

## Notes

{Space for learner notes during session}

---

**Session Complete**: [ ] Yes  [ ] No (continue in next session)

**Date Completed**: ___________

**Evidence Saved**: [ ] Tests  [ ] Coverage  [ ] Notes
