---
phase: 10
slug: protocol-definitions-and-config
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-20
---

# Phase 10 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — standard Go test runner |
| **Quick run command** | `go test ./internal/protocol/... ./internal/config/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/protocol/... ./internal/config/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 10-01-01 | 01 | 1 | PROTO-01 | unit | `go test ./internal/protocol/...` | ❌ W0 | ⬜ pending |
| 10-01-02 | 01 | 1 | PROTO-02 | unit | `go test ./internal/protocol/...` | ❌ W0 | ⬜ pending |
| 10-01-03 | 01 | 1 | NODE-01 | unit | `go test ./internal/config/...` | ❌ W0 | ⬜ pending |
| 10-01-04 | 01 | 1 | NODE-02 | unit | `go test ./internal/config/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/protocol/messages_test.go` — round-trip marshal/unmarshal tests for all message types
- [ ] `internal/config/config_test.go` — extend existing tests for new node config fields

*Existing test infrastructure covers Go test runner — no framework install needed.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Hardware ID derivation on real machine | NODE-01 | Requires actual hardware identifiers | Run `go test ./internal/config/... -run TestNodeID` on target machine |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
