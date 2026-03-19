---
phase: 1
slug: core-bot-infrastructure
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-19
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — Wave 0 installs |
| **Quick run command** | `go test ./... -short -count=1` |
| **Full suite command** | `go test ./... -race -count=1` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./... -short -count=1`
- **After every plan wave:** Run `go test ./... -race -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 01-01-01 | 01 | 1 | CORE-02 | unit | `go test ./internal/config/ -run TestLoadConfig` | ❌ W0 | ⬜ pending |
| 01-01-02 | 01 | 1 | DEPLOY-03 | unit | `go test ./internal/config/ -run TestResolvePaths` | ❌ W0 | ⬜ pending |
| 01-02-01 | 02 | 1 | SESS-01 | unit | `go test ./internal/claude/ -run TestStreamParsesNDJSON` | ❌ W0 | ⬜ pending |
| 01-02-02 | 02 | 1 | SESS-02,SESS-03 | unit | `go test ./internal/claude/ -run TestUnmarshal` | ❌ W0 | ⬜ pending |
| 01-03-01 | 03 | 1 | AUTH-01 | unit | `go test ./internal/security/ -run TestIsAuthorized` | ❌ W0 | ⬜ pending |
| 01-03-02 | 03 | 1 | CORE-05 | unit | `go test ./internal/security/ -run TestRateLimiter` | ❌ W0 | ⬜ pending |
| 01-03-03 | 03 | 1 | AUTH-02,AUTH-03 | unit | `go test ./internal/security/ -run TestValidatePath` | ❌ W0 | ⬜ pending |
| 01-04-01 | 04 | 2 | SESS-04 | unit | `go test ./internal/session/ -run TestNewSession` | ❌ W0 | ⬜ pending |
| 01-04-02 | 04 | 2 | SESS-02 | unit | `go test ./internal/session/ -run TestPersistence` | ❌ W0 | ⬜ pending |
| 01-05-01 | 05 | 1 | SESS-03 | unit | `go test ./internal/formatting/ -run TestConvert` | ❌ W0 | ⬜ pending |
| 01-06-01 | 06 | 3 | CORE-03,CORE-04 | unit | `go test ./internal/bot/ -run TestMiddleware` | ❌ W0 | ⬜ pending |
| 01-07-01 | 07 | 3 | CMD-01..CMD-05 | unit | `go test ./internal/handlers/ -run TestBuildStatus` | ❌ W0 | ⬜ pending |
| 01-07-02 | 07 | 3 | CMD-05 | unit | `go test ./internal/handlers/ -run TestCallback` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `go.mod` — Initialize Go module with dependencies
- [ ] `internal/config/config_test.go` — stubs for CORE-02, DEPLOY-03
- [ ] `internal/claude/events_test.go` — stubs for SESS-01, SESS-02, SESS-03
- [ ] `internal/claude/process_test.go` — stubs for SESS-01
- [ ] `internal/security/ratelimit_test.go` — stubs for CORE-05
- [ ] `internal/security/validate_test.go` — stubs for AUTH-01, AUTH-02, AUTH-03
- [ ] `internal/session/session_test.go` — stubs for SESS-04
- [ ] `internal/session/persist_test.go` — stubs for PERS-01..PERS-03
- [ ] `internal/formatting/markdown_test.go` — stubs for SESS-03
- [ ] `internal/bot/middleware_test.go` — stubs for CORE-03, CORE-04
- [ ] `internal/handlers/command_test.go` — stubs for CMD-01..CMD-05
- [ ] `internal/handlers/callback_test.go` — stubs for CMD-05

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Bot connects to Telegram and receives messages | CORE-01 | Requires live Telegram API token | Start bot, send message from phone, verify response |
| Streaming response visible in Telegram | SESS-02 | Requires live Telegram + Claude CLI | Send text, watch message update live |
| Bot survives restart and restores session | PERS-02 | Requires process restart | Stop bot, restart, verify session resumes via /resume |
| Go binary compiles for Windows | DEPLOY-01 | Requires Windows build environment | Run `go build -o gsd-tele-go.exe .` |
| Graceful shutdown drains sessions | DEPLOY-04 | Requires active session + signal | Send SIGTERM during active query, verify completion |
| Claude/pdftotext paths resolved and logged | DEPLOY-03 | Requires paths on system | Check startup logs for resolved paths |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
