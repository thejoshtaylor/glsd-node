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

## Cross-Milestone Trends

### Process Evolution

| Milestone | Sessions | Phases | Key Change |
|-----------|----------|--------|------------|
| v1.0 | ~20 | 7 | First milestone — established GSD autonomous workflow |

### Cumulative Quality

| Milestone | Tests | Coverage | Zero-Dep Additions |
|-----------|-------|----------|-------------------|
| v1.0 | 77+ | handler-level | 0 (all deps justified) |

### Top Lessons (Verified Across Milestones)

1. Audit early, verify immediately — deferred verification creates rework phases
2. Convergence points in handler chains simplify cross-cutting concerns
