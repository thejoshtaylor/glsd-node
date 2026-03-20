---
phase: 4
slug: callback-handler-integration-fixes
status: compliant
nyquist_compliant: true
wave_0_complete: true
created: 2026-03-19
last_audited: 2026-03-20
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing stdlib |
| **Config file** | none (standard `go test`) |
| **Quick run command** | `"/c/Program Files/Go/bin/go" test ./internal/handlers/... -run TestCallback -v` |
| **Full suite command** | `"/c/Program Files/Go/bin/go" test ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `"/c/Program Files/Go/bin/go" test ./internal/handlers/... -run TestCallback -v`
- **After every plan wave:** Run `"/c/Program Files/Go/bin/go" test ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 04-01-01 | 01 | 0 | DEPLOY-04 | unit | `go test ./internal/handlers/... -run TestEnqueueGsdCommand_UsesInjectedWg` | ✅ | ✅ green |
| 04-01-02 | 01 | 0 | PROJ-01, PROJ-03, PERS-03 | unit | `go test ./internal/handlers/... -run TestCallbackResume_UsesMapping` | ✅ | ✅ green |
| 04-01-03 | 01 | 0 | PROJ-01, PROJ-03, PERS-03 | unit | `go test ./internal/handlers/... -run TestCallbackNew_UsesMapping` | ✅ | ✅ green |
| 04-01-04 | 01 | 0 | CORE-06 | unit | `go test ./internal/handlers/... -run TestEnqueueGsdCommand_GlobalLimiterCompile` | ✅ | ✅ green |
| 04-01-05 | 01 | 1 | ALL | build | `go build ./...` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/handlers/callback_integration_test.go` — regression tests for WaitGroup threading, mapping resolution, and rate limiter pass-through
- [ ] No framework install needed — stdlib `testing` package

*Existing infrastructure covers framework requirements.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Graceful shutdown drains callback workers | DEPLOY-04 | Requires running bot + sending SIGTERM during active callback | 1. Start bot 2. Trigger GSD callback 3. Send SIGTERM 4. Verify log shows "waiting for workers" and clean exit |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 15s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved (2026-03-20)
