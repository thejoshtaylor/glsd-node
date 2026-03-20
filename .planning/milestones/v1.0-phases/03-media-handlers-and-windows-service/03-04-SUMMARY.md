---
phase: "03"
plan: "04"
subsystem: bot-dispatcher, documentation
tags: [handler-wiring, nssm, windows-service, media-handlers]
dependency_graph:
  requires: ["03-02", "03-03"]
  provides: ["media-handler-routing", "nssm-deployment-docs"]
  affects: ["internal/bot/handlers.go", "docs/windows-service.md"]
tech_stack:
  added: []
  patterns: ["bot-wrapper-delegation", "dispatcher-filter-registration"]
key_files:
  modified: ["internal/bot/handlers.go"]
  created: ["docs/windows-service.md"]
decisions:
  - "Media handlers registered after text handler but before command handlers in dispatcher"
  - "NSSM docs recommend AppEnvironmentExtra over AppEnvironment to preserve inherited env"
  - "CreationDisposition 4 (append mode) for NSSM log files"
  - "Checkpoint auto-approved via --auto flag"
metrics:
  duration_minutes: 2
  completed: "2026-03-20T04:05:00Z"
---

# Phase 03 Plan 04: Bot Dispatcher Wiring + NSSM Documentation Summary

Media handler dispatcher registration and NSSM Windows Service installation guide completing Phase 3.

## One-liner

Voice/photo/document handlers wired into bot dispatcher with thin wrapper methods; NSSM Windows Service docs cover install, env vars, logging, and troubleshooting.

## Task Results

### Task 1: Register media handlers in bot dispatcher

**Commit:** `3c144ac`

Added three wrapper methods on Bot (`handleVoice`, `handlePhoto`, `handleDocument`) following the exact pattern of `handleText`. Each delegates to the corresponding `bothandlers.Handle*` function with the same parameter set: `tgBot, ctx, store, cfg, auditLog, persist, WaitGroup(), mappings, globalAPILimiter`.

Registered three new message filters in `registerHandlers()` after the text handler and before command handlers:
- `message.Voice` -> `b.handleVoice`
- `message.Photo` -> `b.handlePhoto`
- `message.Document` -> `b.handleDocument`

All media messages now flow through the existing auth middleware (group -2) and rate limit middleware (group -1) before reaching their handlers.

**Verification:** `go build ./...` exits 0, `go test ./... -count=1` all pass.

### Task 2: Create NSSM Windows Service documentation

**Commit:** `4e772cf`

Created `docs/windows-service.md` with complete sections:
- Prerequisites (build binary, download NSSM, locate tool paths)
- Install Service (6-step process with exact commands)
- Manage Service (status, stop, restart, edit, remove)
- Environment Variables Reference (12 variables with required/optional, defaults)
- Troubleshooting (service won't start, Claude CLI not found, temp directory issues, PDF extraction)

Key details: `AppEnvironmentExtra` (not `AppEnvironment`), `CreationDisposition 4` for append-mode logs, SYSTEM account temp dir workaround, GUI fallback for values with spaces.

### Task 3: Human verification checkpoint

Auto-approved (--auto flag). Build and full test suite verified green.

## Deviations from Plan

None -- plan executed exactly as written.

## Self-Check: PASSED

- FOUND: internal/bot/handlers.go
- FOUND: docs/windows-service.md
- FOUND: 03-04-SUMMARY.md
- FOUND: commit 3c144ac
- FOUND: commit 4e772cf
