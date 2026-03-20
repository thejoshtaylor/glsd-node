---
phase: 5
slug: fix-session-metrics-and-gsd-persistence
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-20
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib) |
| **Config file** | none (standard `go test ./...`) |
| **Quick run command** | `go test ./internal/claude/... ./internal/session/... ./internal/handlers/...` |
| **Full suite command** | `go test ./... -race` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/claude/... ./internal/session/... ./internal/handlers/...`
- **After every plan wave:** Run `go test ./... -race`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 05-01-01 | 01 | 1 | SESS-07 | unit | `go test ./internal/claude/... -run TestLastUsage` | ❌ W0 | ⬜ pending |
| 05-01-02 | 01 | 1 | SESS-06 | unit | `go test ./internal/claude/... -run TestLastContextPercent` | ❌ W0 | ⬜ pending |
| 05-01-03 | 01 | 1 | SESS-07 | unit | `go test ./internal/session/... -run TestProcessMessageCapturesUsage` | ❌ W0 | ⬜ pending |
| 05-01-04 | 01 | 1 | SESS-06 | unit | `go test ./internal/session/... -run TestProcessMessageCapturesContextPercent` | ❌ W0 | ⬜ pending |
| 05-01-05 | 01 | 1 | SESS-07 | unit | `go test ./internal/session/... -run TestProcessMessageNoUsageOnContextLimit` | ❌ W0 | ⬜ pending |
| 05-02-01 | 02 | 1 | PERS-01 | unit | `go test ./internal/handlers/... -run TestEnqueueGsdCommandPersists` | ❌ W0 | ⬜ pending |
| 05-02-02 | 02 | 1 | PERS-01 | integration | `go test ./internal/handlers/... -run TestGsdPersistenceEndToEnd` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/claude/process_test.go` — add TestLastUsage, TestLastContextPercent covering capture from result events
- [ ] `internal/session/session_test.go` — add TestProcessMessageCapturesUsage, TestProcessMessageCapturesContextPercent, TestProcessMessageNoUsageOnContextLimit
- [ ] `internal/handlers/callback_test.go` — add TestEnqueueGsdCommandPersists testing OnQueryComplete fires when worker starts

*Existing infrastructure covers framework and fixtures — no new install needed.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| /status displays real token counts after query | SESS-07 | Requires live Claude CLI subprocess | Send message to bot, run /status, verify non-zero token counts |
| /status displays context percent after query | SESS-06 | Requires live Claude CLI subprocess | Send message to bot, run /status, verify context bar present |
| GSD keyboard session appears in /resume | PERS-01 | Requires Telegram callback interaction | Trigger GSD via inline keyboard, run /resume, verify session listed |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
