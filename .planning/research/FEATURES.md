# Feature Research

**Domain:** Node-to-server WebSocket agent — CLI orchestration node software
**Researched:** 2026-03-20
**Confidence:** HIGH (architecture patterns, existing code reuse); MEDIUM (protocol specifics)

## Scope Note

This document covers v1.2 research. The v1.1 and v1.0 feature landscape is archived in
`.planning/milestones/`. v1.2 is a milestone pivot: remove the Telegram bot interface entirely
and replace it with an outbound WebSocket connection to a central server.

---

## v1.2 Feature Landscape

### Table Stakes (Users Expect These)

Features the server operator expects to exist. Missing these makes the node undeployable or
unreliable in the target architecture.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Outbound WebSocket connection | No firewall changes — node dials server; server does not dial node | MEDIUM | TLS (`wss://`) required in production; gorilla/websocket v1.5.1 or coder/websocket are both viable |
| Node registration on connect | Server must know node identity before dispatching commands | LOW | First frame after handshake: send `node_id`, platform, project list, version; block command handling until ACK received |
| Heartbeat / ping-pong keepalive | Idle connections silently drop at NAT routers and load balancers | LOW | Send ping every 30s; server expects pong within 10s; server closes connection after 90s of silence; standard interval per distributed systems literature |
| Automatic reconnection with exponential backoff | Transient network failures are inevitable; the node must self-heal without operator intervention | MEDIUM | Start at 500ms delay, double each attempt, cap at 30s, add jitter to prevent thundering herd on server restart; re-register on each reconnect |
| Online/offline status detection by server | Server must know which nodes are reachable before dispatching commands | LOW | Inferred from connection state and missed heartbeat threshold; send explicit `disconnect` frame on clean shutdown |
| Command dispatch from server to node | The entire purpose of the system — server sends commands, node executes them | MEDIUM | JSON frames with `type`, `command_id`, `project`, `payload`; node sends ACK then streams results |
| Spawn Claude CLI subprocess on command | Core node action | LOW | `internal/claude/process.go` is reused directly; `NewProcess()` and `Stream()` already handle all subprocess and NDJSON concerns |
| Stream Claude output back to server | Server needs live updates, not only the final result | MEDIUM | Forward each NDJSON event as a WebSocket frame wrapped in an envelope: `{type: "chunk", instance_id: "...", event: {...}}`; send in the same goroutine that reads from the `StatusCallback` |
| Instance lifecycle events | Server must track when a Claude instance starts, finishes, or errors | LOW | Events: `instance_started`, `instance_chunk`, `instance_finished`, `instance_error`; each includes `instance_id` and `project` |
| Multiple simultaneous Claude instances per project | Parallel GSD execution in the same working directory | HIGH | Per-instance goroutine; `map[instanceID]*claude.Process` guarded by mutex; single writer goroutine for all WebSocket output — concurrent `conn.WriteMessage` calls are not safe |
| Instance ID assignment | Server must address specific running instances for kill, query, or correlation | LOW | UUID generated at spawn time; included in every outgoing frame envelope |
| Graceful shutdown | Windows Service stop must not orphan Claude subprocesses | MEDIUM | On SIGTERM/service stop: stop accepting new commands, wait for in-flight streams to finish (with timeout), kill remaining processes with `taskkill /T /F`, send disconnect frame, exit |
| Config via env file | Standard deployment pattern; operators manage `.env` files | LOW | Extend existing `internal/config/config.go` pattern; add `NODE_ID`, `SERVER_URL`, `SERVER_TOKEN`, `HEARTBEAT_INTERVAL_SECS`; remove Telegram/OpenAI vars |
| Remove Telegram dependencies entirely | Stated milestone goal; node software must not carry Telegram coupling | MEDIUM | Remove `internal/bot/`, remove `gotgbot/v2` import, remove Telegram-specific handlers; Claude process management is retained |
| Structured logging with instance context | Operational visibility without console access | LOW | zerolog already in use; extend with `node_id`, `instance_id`, `project` fields on all log entries related to instance lifecycle |
| Audit logging for received commands | Security requirement — record what the server instructed the node to do | LOW | `internal/audit/log.go` is reused; extend log entries with `source: websocket`, `command_type`, `command_id` |

### Differentiators (Competitive Advantage)

Features that make this node system genuinely useful beyond the bare minimum.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Per-project session persistence and resume | Claude sessions survive node restarts; expensive context is not lost | LOW | Already built: session ID captured from result events in `process.go`; persisted to JSON; reloaded on reconnect; pass `--resume SESSION_ID` on next spawn |
| Token and context usage forwarding | Server UI can display context % and token counts per instance | LOW | Already captured: `p.LastContextPercent()`, `p.LastUsage()` from `process.go`; include in `instance_finished` event |
| Per-project working directory isolation | Multiple projects on one node cannot interfere with each other | LOW | Already built in `internal/project/mapping.go`; `cmd.Dir` is set per-project at spawn time |
| Rate limiting on incoming commands | Prevent runaway Claude usage from a misconfigured or compromised server | LOW | Token bucket (`golang.org/x/time/rate`) already in `internal/security`; apply per-project to incoming `run` commands |
| Kill command support | Server can terminate a specific running instance — essential for long-running GSD phases | LOW | Lookup instance by ID in the map; call `p.Kill()`; send `instance_finished` with error reason |
| Streaming chunk throttle (deduplicated) | Avoid flooding the WebSocket with high-frequency NDJSON events from Claude | LOW | Apply a 100ms minimum interval between forwarded chunk frames per instance; batch text content, send tool events immediately — mirrors existing `StreamingState` throttle logic |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Inbound listening port on node | "Simpler" bidirectional — server calls node directly | Breaks the fundamental NAT traversal requirement; requires static IP and firewall rule per node; negates the whole reason for outbound-only design | Outbound WebSocket is the correct pattern: server pushes commands over the established connection; the connection direction and command direction are independent |
| Server-side message persistence and replay queue | "Reliable delivery" — buffer commands when node is offline | Queued commands become stale: a buffered "start GSD phase 2" command issued 10 minutes ago when the node was offline may be wrong to execute on reconnect | Design server to not dispatch commands to offline nodes; node reports live project state on reconnect so server operator can reissue intentionally |
| Binary framing (protobuf, MessagePack) | Smaller payloads, faster serialization | Negligible benefit at this scale (one developer, local network latency); adds schema compilation step and makes log inspection harder; NDJSON is already the Claude CLI format | JSON text frames throughout; log any frame at DEBUG level and it is human-readable |
| Shared Claude sessions across projects | "Efficiency" — one Claude process for multiple projects | Validated out of scope in v1.0: Claude sessions are directory-scoped; mixing projects causes wrong working directory for tool calls and context bleed | One session per project, always — this is a validated key decision |
| HTTP REST fallback transport | "Reliability" — if WebSocket fails, fall back to polling | Doubles the protocol surface area; creates ambiguity about which transport is authoritative; complicates connection state reasoning | Exponential backoff reconnection handles transient WebSocket failures cleanly; no second transport needed |
| Auto-discovery / mDNS / gossip protocol | "Zero config" — nodes find the server automatically | Unnecessary complexity for a single-server deployment; mDNS does not cross subnets; gossip protocols add significant implementation surface | Static `SERVER_URL` in env; nodes register to one known server URL; explicit config is simpler and auditable |
| Node-side web dashboard | "Visibility" without the server | The server is the management plane by design; a local dashboard on the node duplicates that responsibility and adds an HTTP server dependency | All visibility flows through the server WebSocket connection; the node is headless by design |
| Webhook or push registration (node tells server its IP) | Allows server to re-dial the node if connection drops | Reintroduces inbound port requirement and static IP assumption; breaks NAT traversal | The node is responsible for maintaining the connection; it re-connects automatically on drop |

## Feature Dependencies

```
[Outbound WebSocket connection]
    └──enables──> [Node registration on connect]
                      └──enables──> [Command dispatch from server]
                                        └──enables──> [Spawn Claude CLI subprocess]
                                                          └──enables──> [Stream Claude output to server]
                                                                            └──enables──> [Instance lifecycle events]

[Heartbeat / ping-pong]
    └──enables──> [Online/offline status detection]
    └──requires──> [Outbound WebSocket connection]

[Automatic reconnection]
    └──requires──> [Outbound WebSocket connection]
    └──must trigger──> [Node registration on connect] (re-register after each reconnect)

[Multiple simultaneous instances]
    └──requires──> [Instance ID assignment] (envelope routing)
    └──requires──> [Single writer goroutine] (WebSocket concurrency safety)
    └──requires──> [Spawn Claude CLI subprocess]

[Kill command]
    └──requires──> [Instance ID assignment] (to identify which instance to kill)
    └──requires──> [Multiple simultaneous instances] (map of running instances)

[Graceful shutdown]
    └──requires──> [Kill command] (to terminate remaining instances)
    └──requires──> [Instance lifecycle events] (drain before exit)

[Per-project session persistence]  (already built)
    └──enhances──> [Spawn Claude CLI subprocess] (pass --resume SESSION_ID)

[Remove Telegram dependencies]
    └──conflicts with──> [internal/bot/, internal/handlers/streaming.go] (delete, not modify)
    └──preserves──> [internal/claude/, internal/project/, internal/security/, internal/audit/]
```

### Dependency Notes

- **Command dispatch requires registration ACK:** The node must not process any `run` command before the server acknowledges the registration frame. Buffer and discard commands received before ACK.
- **Streaming requires single writer goroutine:** Multiple goroutines (one per Claude instance) must not write to the WebSocket connection concurrently. All outbound frames must be funneled through a single `chan []byte` with a dedicated writer goroutine. This is a correctness requirement, not an optimization.
- **Reconnect always re-registers:** The server treats each new WebSocket connection as a fresh node until registration is received. The node must re-send its registration frame after every reconnect, including project list and current instance state.
- **Kill command is optional in MVP but enables graceful project state management:** Without it, a long-running Claude instance can only be stopped by restarting the node service. Include it in P1 because it is low complexity and high operational value.

## MVP Definition

### Launch With (v1.2)

Minimum viable node that can replace the Telegram bot interface end-to-end.

- [ ] Outbound WebSocket connection (`wss://SERVER_URL`) with TLS — fundamental transport
- [ ] Node registration frame on connect: `node_id`, platform, project list, software version
- [ ] Heartbeat every 30s with pong timeout enforcement — connection health
- [ ] Automatic reconnection with exponential backoff (500ms to 30s, jitter) — self-healing
- [ ] Single writer goroutine for all outbound WebSocket frames — concurrency correctness
- [ ] Command dispatch: receive `run` command, parse project + prompt, spawn Claude CLI, stream output back
- [ ] Instance ID (UUID) assigned at spawn, included in every outgoing envelope
- [ ] Instance lifecycle events: `instance_started`, `instance_chunk`, `instance_finished`, `instance_error`
- [ ] Multiple simultaneous Claude instances per project — concurrent goroutines, shared instance map
- [ ] Kill command: terminate specific instance by ID
- [ ] Graceful shutdown: drain active streams, kill remaining processes, send `disconnect` frame, exit
- [ ] Config via `.env`: `NODE_ID`, `SERVER_URL`, `SERVER_TOKEN`, `HEARTBEAT_INTERVAL_SECS`
- [ ] Remove `internal/bot/`, remove `gotgbot/v2` dependency, remove Telegram-specific code

### Add After Validation (v1.2.x)

Features to add once the node-server link is working end-to-end in production.

- [ ] Status query command — server asks node for list of running instances and their state (trigger: server UI needs real-time dashboard)
- [ ] Token and context usage forwarded in `instance_finished` events (trigger: server UI displays cost/context data)
- [ ] Rate limiting on incoming `run` commands per project (trigger: observed runaway usage or rogue server sends)
- [ ] Streaming chunk throttle — 100ms minimum interval per instance (trigger: observed WebSocket flood from high-verbosity Claude runs)

### Future Consideration (v2+)

Features to defer until the server is built and basic node operation is validated.

- [ ] TLS certificate pinning — pin the server cert for stronger MITM protection (defer: bearer token auth is sufficient for v1; pinning adds deployment friction)
- [ ] Command timeout enforcement on node — kill Claude if it runs beyond N minutes (defer: server can track wall time and issue a kill command; simpler than node-side timeout)
- [ ] Metrics endpoint — Prometheus-compatible process stats: active instances, bytes forwarded, reconnect count (defer: operational need emerges post-deployment)
- [ ] Multiple server connections — node reports to two servers simultaneously for HA failover (defer: single-server model is the whole deployment; adds significant state complexity)
- [ ] Compressed WebSocket frames — `permessage-deflate` for high-volume NDJSON streaming (defer: local network latency dominates; compression benefit is marginal)

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Outbound WebSocket connection | HIGH | MEDIUM | P1 |
| Node registration on connect | HIGH | LOW | P1 |
| Heartbeat / keepalive | HIGH | LOW | P1 |
| Automatic reconnection | HIGH | MEDIUM | P1 |
| Single writer goroutine | HIGH | LOW | P1 — correctness, not optional |
| Command dispatch + Claude spawn | HIGH | MEDIUM | P1 |
| Stream Claude output to server | HIGH | MEDIUM | P1 |
| Instance ID assignment | HIGH | LOW | P1 |
| Instance lifecycle events | HIGH | LOW | P1 |
| Multiple simultaneous instances | HIGH | HIGH | P1 — stated milestone goal |
| Kill command | HIGH | LOW | P1 — low cost, high operational value |
| Graceful shutdown | HIGH | MEDIUM | P1 |
| Remove Telegram dependencies | HIGH | MEDIUM | P1 — stated milestone goal |
| Config via env extension | HIGH | LOW | P1 |
| Status query command | MEDIUM | LOW | P2 |
| Token usage forwarding | LOW | LOW | P2 |
| Rate limiting on commands | MEDIUM | LOW | P2 |
| Streaming chunk throttle | LOW | LOW | P2 |
| TLS certificate pinning | LOW | MEDIUM | P3 |
| Command timeout on node | MEDIUM | MEDIUM | P3 |
| Metrics endpoint | LOW | MEDIUM | P3 |

**Priority key:**
- P1: Must have for v1.2 launch
- P2: Should have, add in v1.2.x after validation
- P3: Future consideration

## Existing Code Reuse Map

v1.2 transforms the bot into headless node software. The Claude CLI management stack is
preserved entirely. Only the Telegram interface layer is removed.

| Existing Module | Reuse in v1.2 | What Changes |
|-----------------|---------------|--------------|
| `internal/claude/process.go` | Full reuse — `NewProcess`, `Stream`, `Kill`, `SessionID`, `LastUsage` | None — this is the node's core value; untouched |
| `internal/claude/events.go` | Full reuse — `ClaudeEvent`, `AssistantMsg`, `UsageData` | None |
| `internal/project/mapping.go` | Full reuse — project name to directory mapping | Remove Telegram channel ID association; projects are keyed by name/slug, not channel ID |
| `internal/config/config.go` | Pattern reuse — env parsing with validation | Remove Telegram/OpenAI vars; add `NODE_ID`, `SERVER_URL`, `SERVER_TOKEN`, `HEARTBEAT_INTERVAL_SECS` |
| `internal/security/` rate limiter | Full reuse — token bucket per project | Apply to incoming WebSocket commands instead of Telegram messages |
| `internal/audit/log.go` | Full reuse — structured audit trail | Extend log entry type with `source: websocket`, `command_type`, `command_id` |
| `internal/handlers/streaming.go` | Concept reuse — `StatusCallback` pattern | Replace Telegram message edit calls with WebSocket frame writes; `StreamingState` becomes `InstanceStream` |
| `internal/bot/` | Delete entirely | Telegram dispatcher, middleware, bot setup — gone |
| `internal/handlers/` (Telegram handlers) | Delete or gut | `command.go`, `text.go`, `voice.go`, `photo.go`, `document.go`, `callback.go` — gone; only `streaming.go` concept survives in new form |
| `internal/formatting/` | Remove or defer | Telegram MarkdownV2 conversion is not needed; plain text forwarding of NDJSON is sufficient |

## Sources

- [Heartbeats in Distributed Systems — Martin Fowler Patterns of Distributed Systems](https://martinfowler.com/articles/patterns-of-distributed-systems/heartbeat.html) — HIGH confidence
- [Understanding the Heartbeat Pattern in Distributed Systems](https://medium.com/@a.mousavi/understanding-the-heartbeat-pattern-in-distributed-systems-5d2264bbfda6) — MEDIUM confidence
- [Go WebSocket Server Guide: coder/websocket vs gorilla](https://websocket.org/guides/languages/go/) — HIGH confidence
- [WebSocket Reconnection: State Sync and Recovery Guide](https://websocket.org/guides/reconnection/) — HIGH confidence
- [WebSocket Architecture Best Practices — Ably](https://ably.com/topic/websocket-architecture-best-practices) — MEDIUM confidence
- [Go Concurrency Patterns: Pipelines and cancellation](https://go.dev/blog/pipelines) — HIGH confidence (official Go blog)
- [Best Practices for Multi-Agent Workflows in Go (2026)](https://dasroot.net/posts/2026/03/best-practices-multi-agent-workflows-go/) — MEDIUM confidence
- [gowscl: Robust WebSocket client with auto-reconnection and exponential backoff](https://github.com/evdnx/gowscl) — MEDIUM confidence (reference implementation)
- Existing codebase: `internal/claude/process.go`, `internal/handlers/streaming.go` — HIGH confidence (direct inspection)
- `.planning/PROJECT.md` requirements and key decisions — HIGH confidence (authoritative project context)

---
*Feature research for: GSD Node v1.2 — WebSocket node software replacing Telegram bot*
*Researched: 2026-03-20*
