# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.0 — GSD Telegram Bot Go Rewrite

**Shipped:** 2026-03-20
**Phases:** 7 | **Plans:** 24 | **Sessions:** ~20

### What Was Built
- Complete Go rewrite of Telegram-to-Claude bridge (11,257 LOC across 50 files)
- Multi-project support: independent Claude CLI sessions per Telegram channel
- Full GSD workflow integration with interactive inline keyboard menus
- Media handling: voice (OpenAI Whisper), photos, PDFs (pdftotext), text/code documents
- Windows Service deployment via NSSM with explicit tool path resolution
- Safety layers: rate limiting, path validation, command safety checks, audit logging across all paths

### What Worked
- Phase-first architecture: building core infrastructure (Phase 1) before features prevented integration nightmares
- Single convergence point pattern (enqueueGsdCommand) made safety hardening trivial in Phase 6
- Wave-based parallel plan execution kept velocity high — most plans completed in 5-20 minutes
- Pure function extraction (parseCallbackData, buildStatusText, GSD extractors) made testing straightforward without live Telegram
- Milestone audit between phases caught 5 integration findings early (Phases 4-6 closed all gaps)

### What Was Inefficient
- Phase 3 was executed but never formally verified — required Phase 7 just to write VERIFICATION.md and update tracking
- Some VALIDATION.md files stayed in draft status through final milestone (Phases 5, 6)
- Phase 1 had 9 plans (too granular) — later phases were more efficient with 1-4 plans each
- Go PATH discovery on Windows required manual intervention in early sessions

### Patterns Established
- StatusCallback factory pattern for all streaming responses (createStatusCallback → session.sendMessageStreaming)
- Interface-based middleware (AuthChecker, RateLimitChecker) for testable bot wiring
- Atomic JSON write-rename for all persistence (sessions, mappings)
- Worker goroutine per session with QueuedMessage.ErrCh for async error propagation
- MediaGroupBuffer with timeout-based album coalescing

### Key Lessons
1. Run milestone audit early and often — the mid-milestone audit found 5 integration gaps that would have been painful to discover post-ship
2. Verify each phase immediately after execution — deferring Phase 3 verification created unnecessary rework
3. Keep plan count per phase low (2-4) — Phase 1's 9 plans were harder to coordinate than Phase 6's 2 plans
4. Safety checks must be designed into the handler pattern, not bolted on — the convergence point pattern worked because it was architected that way

### Cost Observations
- Model mix: ~60% sonnet (planning, research), ~40% opus (execution, verification)
- Sessions: ~20 across full milestone
- Notable: Wave-based parallel execution with multiple subagents was the highest-throughput pattern

---

## Milestone: v1.1 — Bugfixes

**Shipped:** 2026-03-20
**Phases:** 2 | **Plans:** 2

### What Was Built
- Fixed polling timeout race: `RequestOpts.Timeout` (15s) gives HTTP layer 5s headroom over 10s long-poll window
- Channel auth via admin lookup: `GetChatAdministrators` checks if any admin is in `AllowedUsers`, cached 15 minutes
- Echo loop prevention: filters bot's own reflected posts and linked-channel auto-forwards before auth check

### What Worked
- Autonomous mode end-to-end: discuss → plan → execute → verify → audit → complete in one session
- Smart discuss batch tables saved time vs sequential questioning
- Research phase caught key gotgbot API detail (`IsChannelPost()` vs `From == nil`) that would have caused bugs
- Single-plan phases executed fast (5-20 min per plan)

### What Was Inefficient
- Phase 8 was already complete on disk but ROADMAP.md wasn't updated — required manual checkbox fix at autonomous start
- VALIDATION.md Nyquist sign-off not updated by executor — stayed in draft

### Patterns Established
- `ChannelAuthFn` type for injectable channel auth in middleware (same pattern as AuthChecker interface)
- `sync.Map` + inline expiry for simple TTL caching without external deps
- Additive auth branches: new auth paths add conditionals after existing checks, never restructure

### Key Lessons
1. Mark roadmap complete immediately during execute-phase, not after — avoids stale state for autonomous mode
2. Admin lookup auth is superior to channel ID allowlists — zero operator config needed
3. Echo filtering must run before all auth checks — cheapest early exit prevents wasted API calls

### Cost Observations
- Model mix: ~50% sonnet (research, verification, integration check), ~50% opus (planning, execution)
- Sessions: 1 (full autonomous run)
- Notable: Entire milestone completed in a single autonomous session

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Sessions | Phases | Key Change |
|-----------|----------|--------|------------|
| v1.0 | ~20 | 7 | First milestone — established GSD autonomous workflow |
| v1.1 | 1 | 2 | Full autonomous end-to-end — discuss→plan→execute→verify→audit→complete |

### Cumulative Quality

| Milestone | Tests | Coverage | Zero-Dep Additions |
|-----------|-------|----------|-------------------|
| v1.0 | 77+ | handler-level | 0 (all deps justified) |
| v1.1 | 85+ | handler + security | 0 (sync.Map from stdlib) |

### Top Lessons (Verified Across Milestones)

1. Audit early, verify immediately — deferred verification creates rework phases
2. Convergence points in handler chains simplify cross-cutting concerns
3. Additive auth patterns (new branch after existing check) prevent regressions
4. Admin lookup > config-based allowlists when zero-config UX matters
