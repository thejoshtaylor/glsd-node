# Project Research Summary

**Project:** gsd-tele — GSD Node v1.2 (WebSocket Claude CLI Orchestration Node)
**Domain:** Go WebSocket node software — Claude CLI subprocess orchestration, multi-instance management, Windows Service deployment
**Researched:** 2026-03-20 (v1.2 milestone; supersedes v1.0/v1.1 summary)
**Confidence:** HIGH

## Executive Summary

GSD Node v1.2 is a headless node process that replaces the Telegram bot interface with an outbound WebSocket connection to a central server. The core value — managing multiple Claude CLI subprocesses per project directory, streaming their NDJSON output, and persisting session IDs for resume — is already built and transport-agnostic. The transformation is architectural surgery, not a rewrite: delete the Telegram layer (`internal/bot`, `internal/handlers`), add a WebSocket layer (`internal/wsconn`, `internal/protocol`, `internal/dispatch`, `internal/node`), and update the session identity model from `channelID int64` to `instanceID string`. Everything below that seam (`internal/claude`, `internal/session`, `internal/project`, `internal/security`, `internal/audit`) survives with minimal changes. The `claude.StatusCallback` function signature is the transport seam: in v1.1 it drives Telegram API calls; in v1.2 it drives WebSocket frame sends. Neither `internal/claude` nor `internal/session` knows which transport is in use.

The recommended stack is minimal by design. `github.com/coder/websocket` v1.8.14 replaces gorilla/websocket (archived 2022) for concurrent-write-safe WebSocket client operation. `github.com/cenkalti/backoff/v4` provides production-grade exponential reconnection with jitter. `encoding/json` (stdlib) handles the custom protocol. The gotgbot and openai-go dependencies are removed entirely. Net dependency change: add 2, remove 2. The identity model shifts cleanly — the server assigns a UUID per `execute` command; that UUID is the `instance_id` that tags every downstream event and session persistence record. The `StreamingState` abstraction (~350 lines of Telegram edit-in-place throttling, MarkdownV2 conversion, message ID management) is replaced by `wsStreamCallback` (~30 lines) that marshals events and calls `wsconn.Send()`.

The dominant risks are concentrated in the WebSocket client phase and the Telegram removal phase. Concurrent writes panicking the connection, goroutine leaks from missing read deadlines, and reconnection storms are all critical-severity pitfalls that must be addressed before any dispatch or session logic is built on top. The Telegram-coupled identity types (`chatID`, `StatusCallbackFactory`) and session persistence records keyed by Telegram channel ID must be excised cleanly before the WebSocket protocol layer references them — retrofitting identity models is expensive and high-risk. Both risk clusters have clear, code-level prevention strategies documented in PITFALLS.md.

## Key Findings

### Recommended Stack

See `.planning/research/STACK.md` for full details and alternatives considered.

The v1.1 stack is almost entirely preserved. The only dependency-level changes are removing `gotgbot/v2` and `openai-go` and adding `coder/websocket` and `cenkalti/backoff/v4`. Go 1.26.1, `rs/zerolog`, `golang.org/x/time/rate`, `joho/godotenv`, `encoding/json`, and `sync.RWMutex`-based state management are all unchanged. Windows Service deployment via NSSM remains unchanged.

**Core technologies:**
- **Go 1.26.1**: Runtime — goroutines match per-instance concurrent session model; single binary for Windows Service; `context.Context` cancellation integrates with both wsconn and subprocess management
- **`github.com/coder/websocket` v1.8.14**: WebSocket client — concurrent-write-safe (unlike gorilla which panics), first-class context support enabling clean shutdown, zero transitive dependencies; active successor to archived gorilla/websocket
- **`github.com/cenkalti/backoff/v4` v4.3.0**: Reconnection backoff — canonical Go implementation with jitter (prevents thundering herd), context-aware; v4 API fits existing codebase patterns (v5 has different interface, no benefit here)
- **`encoding/json` (stdlib)**: Wire protocol — JSON text frames are debuggable with `websocat`/devtools, sufficient at Claude CLI event rates, consistent with the existing NDJSON pipeline; no binary format needed
- **`golang.org/x/time/rate` (stdlib extension)**: Per-project rate limiting — same token-bucket implementation, now keyed by project name instead of channel ID
- **`rs/zerolog` (unchanged)**: Structured logging — extend with `node_id`, `instance_id`, `project` fields on all instance lifecycle log entries

**What NOT to use:** `gorilla/websocket` (archived 2022, panics on concurrent writes), `nhooyr.io/websocket` (use the `coder/websocket` import path directly), any binary serialization format (MessagePack, Protobuf — premature optimization, loses debuggability), inbound HTTP ports or REST endpoints (violates NAT-traversal design constraint), `cenkalti/backoff/v5` (different API, no concrete benefit for this codebase).

### Expected Features

See `.planning/research/FEATURES.md` for full prioritization matrix and anti-feature rationale.

The v1.2 feature set is tightly constrained by the stated milestone goal: remove the Telegram interface entirely and replace it with a WebSocket node that can receive commands from a server and manage multiple concurrent Claude CLI instances. All P1 features are required to make the node deployable.

**Must have (table stakes — v1.2 launch):**
- Outbound WebSocket connection (`wss://SERVER_URL`) with TLS — fundamental transport
- Node registration frame on connect/reconnect: node ID, platform, project list, software version, running-instance snapshot
- Heartbeat ping/pong every 30s with 90s read deadline enforcement — connection health and dead-connection detection
- Automatic reconnection with exponential backoff (500ms to 30s, with jitter) — self-healing without operator intervention
- Single writer goroutine for all outbound WebSocket frames — correctness requirement, not optional
- Command dispatch: receive `execute` from server, resolve project path, spawn Claude CLI, stream NDJSON output back as `stream_event` frames
- Instance ID (UUID assigned by server) included in every outgoing envelope
- Instance lifecycle events: `instance_started`, `instance_chunk`, `instance_finished`, `instance_error`
- Multiple simultaneous Claude instances managed via `InstanceStore` keyed by UUID
- Kill command: terminate specific instance by ID; low complexity, high operational value
- Graceful shutdown: drain active streams, terminate remaining subprocesses, send `disconnect` frame, exit
- Config extension: `NODE_ID`, `WS_SERVER_URL`, `NODE_SECRET`, `MAX_INSTANCES`; remove `TELEGRAM_BOT_TOKEN`, `TELEGRAM_ALLOWED_USERS`
- Remove `internal/bot/`, `internal/handlers/` (Telegram-specific), `gotgbot/v2` dependency

**Should have (add in v1.2.x after validation):**
- Status query response — server can request running instance snapshot
- Token and context usage forwarded in `instance_finished` events (already captured in `process.go`)
- Per-project rate limiting on incoming `execute` commands
- Streaming chunk throttle (100ms minimum interval per instance) to prevent WebSocket flood

**Defer to v2+:** TLS certificate pinning, node-side command timeout, Prometheus metrics endpoint, multiple server connections for HA failover, compressed WebSocket frames.

**Anti-features to reject:** Inbound listening port on node (breaks NAT traversal), server-side message buffering for offline nodes (stale commands on reconnect are harmful), binary framing (premature optimization), HTTP REST fallback transport (doubles protocol surface), shared Claude sessions across projects (validated out in v1.0), node-side web dashboard (server is the management plane).

### Architecture Approach

See `.planning/research/ARCHITECTURE.md` for full component decomposition, data flows, and build order.

The architecture is a layer replacement. Four new internal packages are added; two are removed; five are modified or preserved. The existing session worker loop, subprocess management, path validation, audit logging, and project mapping logic are all reused without modification to their core logic. The build order is: protocol definitions (no deps) → connection manager (riskiest new infra) → session layer migration → dispatch + stream callback → node lifecycle wiring → protocol spec document.

**Major components:**
1. **`internal/node`** — lifecycle orchestrator; replaces `internal/bot`; owns wsconn, dispatch, instance store, project mappings, audit; manages startup registration, heartbeat, and graceful shutdown
2. **`internal/wsconn`** — outbound WebSocket connection manager; reconnect loop with exponential backoff; single write goroutine draining a buffered `sendCh`; ping/pong with read deadline reset in pong handler; exposes `Send([]byte) error` safe for concurrent callers; non-blocking drop semantics for streaming events
3. **`internal/protocol`** — wire protocol type definitions; all inbound message types (`execute`, `stop`, `new_session`, `status_request`, `project_link`) and outbound types (`node_register`, `node_status`, `stream_event`, `instance_started`, `instance_finished`, `instance_error`); `Envelope{Type, MessageID, Payload json.RawMessage}`
4. **`internal/dispatch`** — thin command router; `switch env.Type` routing to handler methods; holds no state; delegates to InstanceStore, MappingStore, and wsconn.Send; each handler independently testable
5. **`internal/session`** (modified) — `InstanceStore` keyed by `instanceID string` (was `channelID int64`); `Instance` struct (renamed from `Session`); Worker loop logic unchanged; `QueuedMessage` updated to use `InstanceID string` instead of `ChatID int64`
6. **`internal/claude`** (unchanged) — subprocess management, NDJSON streaming, session ID persistence; the node's core value; untouched by this transformation

**Key transport seam — `wsStreamCallback` replacing `StreamingState`:**
```
// v1.1: StreamingState drives Telegram API calls (~350 lines)
// v1.2: wsStreamCallback drives WebSocket frame sends (~30 lines)
func wsStreamCallback(instanceID string, cm *wsconn.ConnectionManager) claude.StatusCallback {
    return func(event claude.ClaudeEvent) error {
        // marshal envelope, call cm.Send(env)
    }
}
```

### Critical Pitfalls

See `.planning/research/PITFALLS.md` for full prevention strategies, warning signs, and recovery costs.

1. **Concurrent WebSocket write panic (WS-2)** — Multiple goroutines (streaming workers, heartbeat ticker) writing to the same connection simultaneously panics gorilla/websocket and corrupts coder/websocket frame streams. Prevention: establish the single-writer goroutine (write-pump) pattern in `internal/wsconn` before wiring any Claude streaming output. Validate with `go test -race ./...` and a concurrent-streaming stress test.

2. **Missing read deadline causes goroutine leak on dead connection (WS-5)** — Without a read deadline, the read goroutine blocks forever when the server disappears silently (NAT timeout, crash without FIN). Node appears connected but sends no heartbeats; goroutine count grows on each reconnect. Prevention: set read deadline to `pingInterval + 10s`; reset it in the pong handler. Address in WebSocket client phase before any other phase builds on it.

3. **Session state divergence after reconnect (WS-4)** — On reconnect, the server receives a registration frame with no knowledge of what was running before disconnect. It may dispatch duplicate commands to already-running instances. Prevention: include a running-instance snapshot in the registration frame; design this into the protocol in Phase 1 before implementing the WebSocket client. This is a protocol contract, not an implementation detail.

4. **Multiple Claude CLI instances writing to the same working directory (MP-1)** — Claude CLI does not use file locking. Concurrent subprocesses in the same directory corrupt `.claude/` session state, causing `--resume` failures. Prevention: "multiple instances per project" means multiple logical sessions queued serially via the existing per-instance `queue` channel — not truly parallel subprocesses in the same directory.

5. **Telegram-coupled identity types leaking into new protocol (TG-1 + TG-2)** — `chatID int64` in `QueuedMessage`, `StatusCallbackFactory` taking a Telegram chat ID, and `sessions.json` keyed by channel ID all create Telegram coupling. Prevention: perform the identity model redesign (`channelID int64 → instanceID string`, `channelID int64 → projectName string`) as the first step of Telegram removal, with a migration script for `sessions.json`. Do this before building the WebSocket dispatch layer.

## Implications for Roadmap

The dependency graph from ARCHITECTURE.md and the pitfall-to-phase mapping from PITFALLS.md both point to the same build order: protocol definitions first (unblocks everything, zero risk), connection manager second (riskiest new infrastructure, must be validated in isolation), Telegram removal and session migration third (identity model must be clean before dispatch references it), dispatch fourth, node lifecycle wiring fifth, spec document last.

### Phase 1: Protocol Definitions and Config Foundation

**Rationale:** `internal/protocol/messages.go` defines all message types and is a dependency of every subsequent phase. `internal/config/config.go` adds WebSocket fields and removes Telegram fields. Neither change has any runtime risk — no external dependencies, no behavior change. This phase also bakes in the session-snapshot-on-reconnect requirement (WS-4 prevention) before any implementation begins.

**Delivers:** All message struct definitions (`Envelope`, `execute`, `node_register`, `stream_event`, `instance_finished`, etc.); round-trip marshal/unmarshal tests for all message types; updated config with new env vars; the protocol spec artifact that defines the server contract.

**Addresses:** Command dispatch, registration, heartbeat, lifecycle events — by defining their wire format before anyone implements them.

**Avoids:** WS-4 (session divergence after reconnect) — the registration message includes running-instance snapshot in the struct from day one, not retrofitted later.

### Phase 2: WebSocket Connection Manager

**Rationale:** This is the riskiest new infrastructure. `internal/wsconn` must be built and validated in isolation before anything depends on it. Establishing the single-writer goroutine pattern and reconnect loop first prevents the two highest-severity pitfalls (WS-2 concurrent write panic, WS-5 goroutine leak) from propagating into dispatch or session logic. A working `ConnectionManager` with a mock server test is the gate for all downstream phases.

**Delivers:** `ConnectionManager` with dial loop, exponential backoff (coder/websocket + cenkalti/backoff/v4), read goroutine, write goroutine with `sendCh chan []byte`, ping/pong with read deadline reset in pong handler, bounded channel with drop-on-full semantics for streaming events. Mock server tests for reconnect, send serialization, and dead-connection detection.

**Uses:** `github.com/coder/websocket` v1.8.14, `github.com/cenkalti/backoff/v4` v4.3.0, existing `context.Context` cancellation pattern.

**Avoids:** WS-1 (reconnection storm — exponential backoff with jitter from day one), WS-2 (concurrent write panic — single-writer goroutine), WS-3 (unbounded send channel OOM — bounded channel, drop-on-full), WS-5 (goroutine leak — read deadline + pong handler).

### Phase 3: Telegram Removal and Session Layer Migration

**Rationale:** The identity model must be cleaned up before the WebSocket dispatch layer references it. Carrying `chatID int64` into v1.2 dispatch code creates Telegram coupling that is expensive to remove after tests are written against it. This phase performs the clean-break refactor: delete `internal/bot`, delete Telegram-specific handlers, remove `gotgbot/v2` from `go.mod`, migrate `SessionStore → InstanceStore` (key: `string`), update `QueuedMessage`, audit all middleware functionality, and write the `sessions.json` migration script.

**Delivers:** Codebase with zero Telegram imports in the transport layer; `InstanceStore` keyed by `instanceID string`; `sessions.json` migration from channel-ID keys to project-name keys; audit checklist of all middleware functionality (rate limiting, auth, audit logging, dispatch ordering) with v1.2 equivalents verified.

**Avoids:** TG-1 (Telegram types in protocol layer), TG-2 (persistence key migration breaking session resume), TG-3 (removing dependency before reimplementing all functionality it enabled).

### Phase 4: Dispatch and Stream Callback

**Rationale:** With protocol types defined (Phase 1), wsconn working (Phase 2), and session layer migrated (Phase 3), dispatch is straightforward. A `switch` on envelope type routes to handler methods. `wsStreamCallback` (~30 lines) replaces `StreamingState` (~350 lines). Each handler is independently unit-testable with mock stores.

**Delivers:** `internal/dispatch.Dispatcher` with handlers for `execute`, `stop`, `new_session`, `status_request`, `project_link`, `project_unlink`; `wsStreamCallback` function; path validation via existing `security.ValidatePath`; per-project rate limiting via existing `security.RateLimiter` (keyed by project name); unit tests per command type with mock InstanceStore and mock wsconn.

**Implements:** Dispatch component from architecture; StatusCallback transport bridge pattern; security validation at the dispatch layer.

**Avoids:** MP-1 (concurrent writes to same project directory) — `handleExecute` routes through the per-instance queue, not a direct subprocess spawn.

### Phase 5: Node Lifecycle and End-to-End Wiring

**Rationale:** With all components built and tested in isolation, `internal/node` wires them together. Update `main.go` to instantiate `Node` instead of `Bot`. End-to-end verification against a local mock server confirms the full pipeline: registration → execute → stream_event → instance_finished → disconnect.

**Delivers:** `internal/node.Node` struct wiring wsconn + dispatch + instance store + mapping store + audit + heartbeat; startup sequence (load config → start wsconn → send registration → start heartbeat → block on OS signal); graceful shutdown (cancel context → wait for workers → kill remaining processes → send disconnect frame → exit). Updated `main.go`. Full "looks done but isn't" checklist verification (concurrent write test, dead-connection detection, backpressure test, clean shutdown verification).

**Avoids:** MP-2 (subprocess zombie on cancellation) — shutdown verifies `claude.exe` absent from `tasklist` within 10s; MP-3 (session ID collision on concurrent completion) — per-instance session ID storage confirmed.

### Phase 6: Protocol Spec and Server Interface Document

**Rationale:** Writing the wire protocol spec after the node is working means the spec reflects the actual implementation, not a pre-implementation guess. The server repo needs this document to implement the server side.

**Delivers:** Wire protocol spec (all message types, Envelope format, authentication handshake sequence, sequencing guarantees, reconnect behavior); server interface spec (what WebSocket endpoint and message sequences the server must expose). These are deliverables for the server repo, not code.

### Phase Ordering Rationale

- Protocol first: zero-risk, high-value, unblocks every other phase; the session-state-on-reconnect field is designed in, not bolted on.
- Connection manager second: riskiest new infrastructure; validating it in isolation prevents WS-1, WS-2, WS-3, and WS-5 from propagating into higher layers.
- Telegram removal third: the identity model must be clean before dispatch builds against it; this is the pitfall research's most emphatic finding about retrofit cost.
- Dispatch fourth: can only be built once all three prior phases are stable; each handler is then independently testable.
- Node lifecycle fifth: pure wiring of already-validated components; the risk in this phase is integration, not any individual component.
- Spec document last: reflects reality, not intention; the server team has a working reference implementation to test against.

### Research Flags

Phases likely needing deeper research during planning:

- **Phase 2 (WebSocket Connection Manager):** The coder/websocket API details for read deadline management, ping/pong handler registration, and graceful close sequences should be verified against the library source before writing connection lifecycle code. Edge cases in the reconnect loop (context cancellation during backoff sleep, connection drop during handshake, half-open TCP) warrant specific test case design.
- **Phase 3 (Telegram Removal):** The middleware functionality audit (rate limiting, audit logging, auth, dispatch ordering) requires a line-by-line read of `internal/bot/middleware.go` and all handler files before any deletion begins. The audit checklist is the gate for safe removal.

Phases with standard patterns (skip research-phase):

- **Phase 1 (Protocol + Config):** Defining Go structs and updating env parsing; no unknowns.
- **Phase 4 (Dispatch):** A `switch` statement routing to handler methods; the simplest possible dispatch mechanism; direct template in existing `internal/bot/middleware.go`.
- **Phase 5 (Node Lifecycle):** Wiring well-understood components with `context.Context` cancellation and `sync.WaitGroup`; the existing `internal/bot` is the direct structural template.
- **Phase 6 (Spec Document):** Writing prose documentation from working code; no research needed.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Core libraries verified against pkg.go.dev and GitHub releases with specific version numbers confirmed. coder/websocket v1.8.14 (Sep 2025) and cenkalti/backoff v4.3.0 (Jan 2024) both verified. Library alternatives evaluated with sourced rationale. |
| Features | HIGH | v1.2 feature set is tightly constrained by the existing codebase (direct read) and stated milestone goals. Feature dependency graph is deterministic. Anti-feature rationale is consistent across all three research documents. |
| Architecture | HIGH | Architecture derived primarily from direct codebase read (v1.1, ~11,600 lines Go). Component boundaries, data flow, and the StatusCallback transport seam are verified against actual code. Build order reflects real dependency graph with no assumed dependencies. |
| Pitfalls | HIGH (WebSocket), HIGH (Telegram removal), MEDIUM (multi-instance concurrent subprocess) | WebSocket pitfalls verified against official library docs and GitHub issues. Telegram removal pitfalls derived from direct codebase read. The MP-1 concurrent-directory severity depends on server dispatch behavior — if the server serializes per-project, the risk is lower than if it sends concurrent commands to the same project. |

**Overall confidence:** HIGH

### Gaps to Address

- **Server authentication handshake sequence:** `NODE_SECRET` is defined in config, but the exact protocol for sending it (first frame content, server ACK format, timeout for unauthenticated connections) is not fully specified. This must be pinned down during Phase 1 protocol design or coordinated with the server team before Phase 2 begins. The pitfall research recommends sending auth in the first protocol message after connection, not in the WebSocket URL.

- **Actual server dispatch semantics for concurrent instances:** The MP-1 pitfall (concurrent Claude CLI processes in same directory) assumes the server may send concurrent `execute` commands to the same project. If the server serializes per-project dispatch, the per-directory queue is redundant but harmless. If not, the queue is the sole protection. Clarify the server's intended dispatch behavior before finalizing the `handleExecute` implementation in Phase 4.

- **`MAX_INSTANCES` rejection handling:** When the node rejects an `execute` command with `instance_error{reason:"node at capacity"}`, the server must handle the error response. Confirm the server's retry or queueing strategy before shipping Phase 5 — dropped commands are a silent failure mode without this alignment.

- **coder/websocket read deadline API:** The pitfall research identifies the pattern (set deadline, reset in pong handler) using gorilla/websocket method names. The coder/websocket API for setting read deadlines may differ. Verify the exact method names on `*websocket.Conn` at the start of Phase 2 planning.

- **Session persistence migration for existing data:** The `sessions.json` migration from channel-ID keys to project-name keys requires `mappings.json` as the lookup table. This is straightforward for projects that have channel mappings, but any sessions for channels without a corresponding mapping entry will be unrecoverable. A migration script must handle this gracefully (skip unmappable entries, log what was lost) and be tested on a copy of production data before the first v1.2 deployment.

## Sources

### Primary (HIGH confidence)
- Existing codebase direct read (`internal/claude/process.go`, `internal/session/session.go`, `internal/session/store.go`, `internal/bot/middleware.go`, `internal/bot/bot.go`) — architecture patterns, existing interfaces, subprocess management, session persistence, rate limiting implementation
- [gotgbot v2 pkg.go.dev](https://pkg.go.dev/github.com/PaulSonOfLars/gotgbot/v2@v2.0.0-rc.34) — v1.1 API verification (Sender struct, BaseBotClient, PollingOpts)
- [gotgbot GitHub v2 branch](https://github.com/PaulSonOfLars/gotgbot) — `sender.go`, `request.go`, `ext/context.go`, middleware sample
- [coder/websocket pkg.go.dev](https://pkg.go.dev/github.com/coder/websocket) — v1.8.14 published Sep 5, 2025
- [coder/websocket GitHub releases](https://github.com/coder/websocket/releases) — v1.8.14 release date confirmed
- [cenkalti/backoff v4 pkg.go.dev](https://pkg.go.dev/github.com/cenkalti/backoff/v4) — v4.3.0, published Jan 2, 2024
- [Go downloads page](https://go.dev/dl/) — Go 1.26.1 confirmed latest stable as of March 2026
- [gorilla/websocket concurrent write issues](https://github.com/gorilla/websocket/issues/390) — WS-2 pitfall: "panic: concurrent write to websocket connection"
- [Go blog: Pipelines and cancellation](https://go.dev/blog/pipelines) — concurrency patterns (official Go blog)

### Secondary (MEDIUM confidence)
- [websocket.org Go guide](https://websocket.org/guides/languages/go/) — coder/websocket vs gorilla comparison; concurrent write safety
- [websocket.org reconnection guide](https://websocket.org/guides/reconnection/) — WS-4 state divergence after reconnect
- [Ably: WebSocket architecture best practices](https://ably.com/topic/websocket-architecture-best-practices) — heartbeat and backpressure patterns
- [Ably: WebSocket authentication](https://ably.com/blog/websocket-authentication) — token-in-first-message vs URL parameter
- [cenkalti/backoff v5 pkg.go.dev](https://pkg.go.dev/github.com/cenkalti/backoff/v5) — v5 API comparison; confirmed different from v4
- [Go Forum WebSocket 2025](https://forum.golangbridge.org/t/websocket-in-2025/38671) — community consensus on library choice
- [Martin Fowler: Heartbeat Pattern](https://martinfowler.com/articles/patterns-of-distributed-systems/heartbeat.html) — heartbeat interval and timeout standards
- [betterstack Go logging comparison](https://betterstack.com/community/guides/logging/best-golang-logging-libraries/) — zerolog performance rationale
- [Backpressure in WebSocket streams](https://skylinecodes.substack.com/p/backpressure-in-websocket-streams) — WS-3 pitfall: unbounded buffer and OOM
- [Go graceful shutdown patterns](https://dev.to/yanev/a-deep-dive-into-graceful-shutdown-in-go-484a) — context cancellation, WaitGroup, subprocess cleanup

### Tertiary (LOW confidence)
- [dasroot.net: Best Practices for Multi-Agent Workflows in Go (2026)](https://dasroot.net/posts/2026/03/best-practices-multi-agent-workflows-go/) — directional reference for multi-agent patterns only
- [DEV: JSON vs MessagePack vs Protobuf benchmarks](https://dev.to/devflex-pro/json-vs-messagepack-vs-protobuf-in-go-my-real-benchmarks-and-what-they-mean-in-production-48fh) — serialization tradeoff directional rationale

---
*Research completed: 2026-03-20 (v1.2 — WebSocket node transformation)*
*Supersedes: v1.0 summary (2026-03-19) and v1.1 summary (2026-03-20)*
*Ready for roadmap: yes*
