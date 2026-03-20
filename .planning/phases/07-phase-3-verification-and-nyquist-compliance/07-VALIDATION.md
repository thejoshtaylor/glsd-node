---
phase: 07
slug: phase-3-verification-and-nyquist-compliance
status: draft
nyquist_compliant: false
wave_0_complete: true
created: 2026-03-20
---

# Phase 07 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go test tooling |
| **Quick run command** | `go test ./... -count=1` |
| **Full suite command** | `go test ./... -count=1` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./... -count=1`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 07-01-01 | 01 | 1 | MEDIA-01..05, DEPLOY-02 | file-check | `test -f .planning/phases/03-*/03-VERIFICATION.md` | ✅ | ⬜ pending |
| 07-01-02 | 01 | 1 | — | file-check | `grep "roadmap_complete.*true" .planning/ROADMAP.md` | ✅ | ⬜ pending |
| 07-01-03 | 01 | 1 | — | file-check | `grep "nyquist_compliant: true" .planning/phases/03-*/*-VALIDATION.md .planning/phases/04-*/*-VALIDATION.md` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing go test infrastructure covers all phase requirements. This phase produces documentation artifacts only — no new test files needed.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Windows Service installs and starts | DEPLOY-02 | Requires Windows host with NSSM | Run install script, verify service runs |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
