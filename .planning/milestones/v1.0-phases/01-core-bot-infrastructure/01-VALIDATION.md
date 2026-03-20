---
phase: 1
slug: core-bot-infrastructure
status: compliant
nyquist_compliant: true
wave_0_complete: true
created: 2026-03-19
last_audited: 2026-03-19
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none |
| **Quick run command** | `go test ./... -short -count=1` |
| **Full suite command** | `go test ./... -race -count=1` |
| **Estimated runtime** | ~18 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./... -short -count=1`
- **After every plan wave:** Run `go test ./... -race -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 18 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 01-01-01 | 01 | 1 | CORE-02 | unit | `go test ./internal/config/ -run TestLoadConfig` | ✅ | ✅ green |
| 01-01-02 | 01 | 1 | DEPLOY-03 | unit | `go test ./internal/config/ -run TestResolvePaths` | ✅ | ✅ green |
| 01-02-01 | 02 | 1 | SESS-01 | unit | `go test ./internal/claude/ -run TestStreamParsesNDJSON` | ✅ | ✅ green |
| 01-02-02 | 02 | 1 | SESS-02,SESS-03 | unit | `go test ./internal/claude/ -run TestUnmarshal` | ✅ | ✅ green |
| 01-03-01 | 03 | 1 | AUTH-01 | unit | `go test ./internal/security/ -run TestIsAuthorized` | ✅ | ✅ green |
| 01-03-02 | 03 | 1 | CORE-05 | unit | `go test ./internal/security/ -run TestRateLimiter` | ✅ | ✅ green |
| 01-03-03 | 03 | 1 | AUTH-02,AUTH-03 | unit | `go test ./internal/security/ -run TestValidatePath` | ✅ | ✅ green |
| 01-04-01 | 04 | 2 | SESS-04 | unit | `go test ./internal/session/ -run TestNewSession` | ✅ | ✅ green |
| 01-04-02 | 04 | 2 | SESS-02 | unit | `go test ./internal/session/ -run TestPersistence` | ✅ | ✅ green |
| 01-05-01 | 05 | 1 | SESS-03 | unit | `go test ./internal/formatting/ -run TestConvert` | ✅ | ✅ green |
| 01-06-01 | 06 | 3 | CORE-03,CORE-04 | unit | `go test ./internal/bot/ -run TestMiddleware` | ✅ | ✅ green |
| 01-07-01 | 07 | 3 | CMD-01..CMD-05 | unit | `go test ./internal/handlers/ -run TestBuildStatus` | ✅ | ✅ green |
| 01-07-02 | 07 | 3 | CMD-05 | unit | `go test ./internal/handlers/ -run TestCallback` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `go.mod` — Initialize Go module with dependencies
- [x] `internal/config/config_test.go` — 13 tests covering CORE-02, DEPLOY-03
- [x] `internal/claude/events_test.go` — 9 tests covering SESS-01, SESS-02, SESS-03
- [x] `internal/claude/process_test.go` — 6 tests covering SESS-01, SESS-08
- [x] `internal/security/ratelimit_test.go` — 3 tests covering CORE-05
- [x] `internal/security/validate_test.go` — 12 tests covering AUTH-01, AUTH-02, AUTH-03
- [x] `internal/session/session_test.go` — 7 tests covering SESS-04, SESS-05
- [x] `internal/session/persist_test.go` — 10 tests covering PERS-01..PERS-03
- [x] `internal/formatting/markdown_test.go` — 14 tests covering SESS-02 (MarkdownV2)
- [x] `internal/formatting/tools_test.go` — 15 tests covering SESS-03 (tool emoji)
- [x] `internal/audit/log_test.go` — 3 tests covering CORE-06
- [x] `internal/bot/middleware_test.go` — 5 tests covering CORE-03, CORE-04, AUTH-01
- [x] `internal/handlers/command_test.go` — 12 tests covering CMD-01..CMD-05, SESS-06, SESS-07
- [x] `internal/handlers/callback_test.go` — 18 tests covering CMD-05, callback routing

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

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 18s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** compliant

---

## Validation Audit 2026-03-19

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |

All 13 tasks in the Per-Task Verification Map have corresponding test files with substantive test functions. All 8 test packages pass (`go test ./internal/... -short -count=1` — all OK). No gaps to fill.

**Test inventory:** 127 test functions across 13 test files covering all 28 Phase 1 requirements (automated) plus 6 manual-only items.
