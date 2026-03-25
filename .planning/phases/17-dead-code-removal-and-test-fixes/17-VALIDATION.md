---
phase: 17
slug: dead-code-removal-and-test-fixes
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-24
---

# Phase 17 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing infrastructure |
| **Quick run command** | `go test ./internal/security/... ./internal/dispatch/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/security/... ./internal/dispatch/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 17-01-01 | 01 | 1 | CLEAN-02 | unit | `go test ./internal/security/...` | ✅ | ⬜ pending |
| 17-01-02 | 01 | 1 | CLEAN-02 | build | `go build ./...` | ✅ | ⬜ pending |
| 17-01-03 | 01 | 1 | CLEAN-04 | race | `go test -race ./internal/dispatch/...` | ✅ | ⬜ pending |
| 17-01-04 | 01 | 1 | CLEAN-04 | unit | `go test ./internal/security/... -run TestValidatePathWindowsTraversal` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements.

---

## Manual-Only Verifications

All phase behaviors have automated verification.

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
