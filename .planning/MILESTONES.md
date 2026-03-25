# Milestones

## v1.2 Custom Webapp (Shipped: 2026-03-25)

**Phases completed:** 8 phases, 15 plans, 19 tasks

**Key accomplishments:**

- Envelope dispatcher + all 8 wire message structs with round-trip JSON tests using stdlib only
- Hardware-derived NodeID via machineid with hostname-sha256 fallback, plus standalone NodeConfig parsing SERVER_URL/SERVER_TOKEN without any Telegram dependency
- One-liner:
- `internal/connection/register.go`
- Config struct stripped to node-only fields, TypeScript artifacts deleted, gotgbot removed, main.go rewritten as minimal placeholder with TODO(phase-13) marker
- One-liner:
- Node-oriented audit.Event with Source/NodeID/InstanceID/Project fields, string-keyed ProjectRateLimiter, and exported protocol.NewMsgID() with TypeACK/ACK struct
- One-liner:
- One-liner:
- Complete WebSocket wire protocol spec with 10 message types, 5 Mermaid sequence diagrams, auth handshake, reconnect backoff, and concurrency model -- all derived from working node code
- NodeConfig.Projects field reads PROJECTS env var and flows through sendRegister so the NodeRegister frame carries the real project list instead of a hard-coded empty slice
- InstanceFinished now carries real OS exit code via exec.ExitError extraction and SessionID from proc.SessionID() for server-side resume capability
- One-liner:

---

## v1.1 Bugfixes (Shipped: 2026-03-20)

**Phases completed:** 2 phases, 2 plans, 3 tasks

**Key accomplishments:**

- Added RequestOpts.Timeout (15s) to gotgbot GetUpdatesOpts, eliminating context deadline exceeded errors during idle long-polling by giving the HTTP layer 5 seconds of headroom over the 10s Telegram poll window.
- One-liner:

---

## v1.0 GSD Telegram Bot Go Rewrite (Shipped: 2026-03-20)

**Phases completed:** 7 phases, 24 plans, 10 tasks

**Key accomplishments:**

- (none recorded)

---
