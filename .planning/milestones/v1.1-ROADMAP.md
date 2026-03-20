# Roadmap: GSD Telegram Bot (Go Rewrite)

## Milestones

- v1.0 GSD Telegram Bot Go Rewrite — Phases 1-7 (shipped 2026-03-20) — [Archive](milestones/v1.0-ROADMAP.md)
- v1.1 Bugfixes — Phases 8-9 (in progress)

## Phases

<details>
<summary>v1.0 GSD Telegram Bot Go Rewrite (Phases 1-7) - SHIPPED 2026-03-20</summary>

See [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md) for full phase details.

</details>

### v1.1 Bugfixes (In Progress)

**Milestone Goal:** Fix auth failures in Telegram channels and resolve polling timeout errors for stable daily use.

- [x] **Phase 8: Polling Stability** - Fix HTTP client timeout so long-poll cycles complete without errors
- [x] **Phase 9: Channel Auth** - Fix channel sender auth and filter reflected bot messages (completed 2026-03-20)

## Phase Details

### Phase 8: Polling Stability
**Goal**: Long-polling operates without spurious errors under normal idle conditions
**Depends on**: Phase 7 (v1.0 complete)
**Requirements**: POLL-01
**Success Criteria** (what must be TRUE):
  1. Bot runs for an extended idle period with no `context deadline exceeded` errors in the log
  2. Polling timeout (RequestOpts) is set longer than getUpdates Timeout so HTTP client never races the long-poll window
  3. The timeout change is scoped to the polling call only — sendMessage and editMessage timeouts are unchanged
**Plans:** 1 plan
Plans:
- [x] 08-01-PLAN.md — Add RequestOpts.Timeout to long-poll GetUpdatesOpts

### Phase 9: Channel Auth
**Goal**: Users can operate the bot from Telegram channels without auth rejections, and the bot does not echo-loop its own channel messages
**Depends on**: Phase 8
**Requirements**: AUTH-01, AUTH-02
**Success Criteria** (what must be TRUE):
  1. A message sent in an authorized Telegram channel by the channel itself (non-user sender) is accepted and processed by the bot
  2. A message sent in an unauthorized channel is rejected without the bot posting a visible rejection message into the channel timeline
  3. The bot does not process its own messages reflected back as ChannelPost updates (no echo loop)
  4. Private-chat and group messages from authorized human users continue to pass auth without any behavior change
**Plans:** 1/1 plans complete
Plans:
- [x] 09-01-PLAN.md — Add echo filter, channel auth via admin lookup with cache

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 8. Polling Stability | v1.1 | 1/1 | Complete | 2026-03-20 |
| 9. Channel Auth | v1.1 | 1/1 | Complete   | 2026-03-20 |
