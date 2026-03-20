---
phase: 2
slug: multi-project-and-gsd-integration
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-19
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib) |
| **Config file** | none — go test ./... discovers all *_test.go |
| **Quick run command** | `go test ./internal/handlers/... ./internal/project/...` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/handlers/... ./internal/project/...`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 02-01-01 | 01 | 1 | PROJ-01 | unit | `go test ./internal/project/... -run TestMappingGet` | ❌ W0 | ⬜ pending |
| 02-01-02 | 01 | 1 | PROJ-04 | unit | `go test ./internal/project/... -run TestMappingPersistence` | ❌ W0 | ⬜ pending |
| 02-01-03 | 01 | 1 | PROJ-05 | unit | `go test ./internal/project/... -run TestMappingReassign` | ❌ W0 | ⬜ pending |
| 02-02-01 | 02 | 1 | PROJ-02 | unit | `go test ./internal/handlers/... -run TestWorkerConfigPerProject` | ❌ W0 | ⬜ pending |
| 02-02-02 | 02 | 1 | PROJ-03 | unit | `go test ./internal/handlers/... -run TestHandleTextUnmapped` | ❌ W0 | ⬜ pending |
| 02-03-01 | 03 | 2 | GSD-01 | unit | `go test ./internal/handlers/... -run TestCallbackGsd` | ❌ W0 | ⬜ pending |
| 02-03-02 | 03 | 2 | GSD-02 | unit | `go test ./internal/handlers/... -run TestExtractGsdCommands` | ❌ W0 | ⬜ pending |
| 02-03-03 | 03 | 2 | GSD-03 | unit | `go test ./internal/handlers/... -run TestExtractOptions` | ❌ W0 | ⬜ pending |
| 02-03-04 | 03 | 2 | GSD-04 | unit | `go test ./internal/handlers/... -run TestParseRoadmap` | ❌ W0 | ⬜ pending |
| 02-03-05 | 03 | 2 | GSD-05 | unit | `go test ./internal/handlers/... -run TestAskUserCallback` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/project/mapping_test.go` — stubs for PROJ-01, PROJ-04, PROJ-05
- [ ] `internal/handlers/gsd_test.go` — stubs for GSD-01, GSD-02, GSD-03, GSD-04, GSD-05
- [ ] `internal/handlers/text_test.go` (extend) — stubs for PROJ-02, PROJ-03

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Two channels stream simultaneously without context bleed | PROJ-02 | Requires two live Telegram channels + real Claude sessions | Open two channels, send messages in both, verify responses are project-specific |
| GSD keyboard renders correctly on mobile | GSD-01 | Visual rendering depends on Telegram client | Tap /gsd on phone, verify 8x2 grid + quick-actions row |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
