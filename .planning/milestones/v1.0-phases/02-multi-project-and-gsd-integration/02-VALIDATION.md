---
phase: 2
slug: multi-project-and-gsd-integration
status: approved
nyquist_compliant: true
wave_0_complete: true
created: 2026-03-19
validated: 2026-03-19
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
| **Estimated runtime** | ~4 seconds |

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
| 02-01-01 | 01 | 1 | PROJ-01 | unit | `go test ./internal/project/... -run TestMappingGetEmpty\|TestMappingSetAndGet` | ✅ | ✅ green |
| 02-01-02 | 01 | 1 | PROJ-04 | unit | `go test ./internal/project/... -run TestMappingPersistence` | ✅ | ✅ green |
| 02-01-03 | 01 | 1 | PROJ-05 | unit | `go test ./internal/project/... -run TestMappingReassign` | ✅ | ✅ green |
| 02-02-01 | 02 | 1 | PROJ-02 | unit | `go test ./internal/handlers/... -run TestWorkerConfigPerProject` | ✅ | ✅ green |
| 02-02-02 | 02 | 1 | PROJ-03 | unit | `go test ./internal/handlers/... -run TestHandleTextUnmapped` | ✅ | ✅ green |
| 02-03-01 | 03 | 2 | GSD-01 | unit | `go test ./internal/handlers/... -run TestParseCallbackGsd\|TestBuildGsdKeyboard_RowCount\|TestBuildGsdStatusHeader` | ✅ | ✅ green |
| 02-03-02 | 03 | 2 | GSD-02 | unit | `go test ./internal/handlers/... -run TestExtractGsdCommands` | ✅ | ✅ green |
| 02-03-03 | 03 | 2 | GSD-03 | unit | `go test ./internal/handlers/... -run TestExtractNumberedOptions\|TestExtractLetteredOptions` | ✅ | ✅ green |
| 02-03-04 | 03 | 2 | GSD-04 | unit | `go test ./internal/handlers/... -run TestParseRoadmap` | ✅ | ✅ green |
| 02-03-05 | 03 | 2 | GSD-05 | unit | `go test ./internal/handlers/... -run TestAskUserCallbackTempFile\|TestParseCallbackAskUser` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements. All test files were created during plan execution:

- [x] `internal/project/mapping_test.go` — 7 tests covering PROJ-01, PROJ-04, PROJ-05
- [x] `internal/handlers/gsd_test.go` — 16 tests covering GSD-02, GSD-03, GSD-04
- [x] `internal/handlers/callback_test.go` — covers GSD-01, GSD-05 (plus 10 parse tests)
- [x] `internal/handlers/text_test.go` — covers PROJ-02, PROJ-03

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Two channels stream simultaneously without context bleed | PROJ-02 | Requires two live Telegram channels + real Claude sessions | Open two channels, send messages in both, verify responses are project-specific |
| GSD keyboard renders correctly on mobile | GSD-01 | Visual rendering depends on Telegram client | Tap /gsd on phone, verify 8x2 grid + quick-actions row |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 10s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-03-19

---

## Validation Audit 2026-03-19

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |

All 10 requirements had existing automated tests at audit time. No auditor spawn required.
