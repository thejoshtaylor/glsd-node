---
phase: 06
slug: cross-phase-safety-hardening
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-20
---

# Phase 06 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go test tooling |
| **Quick run command** | `go test ./internal/handlers/ -run TestCallback -count=1` |
| **Full suite command** | `go test ./... -count=1` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/handlers/ -run TestCallback -count=1`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 06-01-01 | 01 | 1 | CORE-03 | unit | `go test ./internal/handlers/ -run TestTyping -count=1` | ❌ W0 | ⬜ pending |
| 06-01-02 | 01 | 1 | CORE-06 | unit | `go test ./internal/handlers/ -run TestAudit -count=1` | ❌ W0 | ⬜ pending |
| 06-01-03 | 01 | 1 | AUTH-03 | unit | `go test ./internal/handlers/ -run TestSafety -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/handlers/callback_test.go` — test stubs for typing, audit, and safety check integration
- [ ] Existing test infrastructure covers framework needs

*Existing go test infrastructure covers all phase requirements.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Typing indicator visible in Telegram | CORE-03 | Requires live Telegram client | Send a GSD button command, observe "typing..." in chat |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
