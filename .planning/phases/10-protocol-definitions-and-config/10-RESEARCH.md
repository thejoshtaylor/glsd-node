# Phase 10: Protocol Definitions and Config - Research

**Researched:** 2026-03-20
**Domain:** Go struct design, JSON envelope protocol, hardware-derived node identity, config extension
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None — all implementation choices are at Claude's discretion for this pure infrastructure phase.

### Claude's Discretion
- Message struct field naming and JSON tags
- Envelope wrapper design (type discriminator approach)
- Config parsing strategy (extend existing `internal/config/` or new package)
- Test structure for round-trip marshal/unmarshal
- Hardware ID derivation method for node identity

### Deferred Ideas (OUT OF SCOPE)
None.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| PROTO-01 | Node sends registration frame on connect: node_id (hardware-derived), platform, project list, version | `denisbrodbeck/machineid` for node_id; `node_register` struct with `running_instances` snapshot field |
| PROTO-02 | Message envelope format: `type` + `id` + JSON payload for all frames | `json.RawMessage` two-stage unmarshal pattern; `Envelope` struct with `Type`, `ID`, `Payload json.RawMessage` |
| NODE-01 | Node ID auto-derived from hardware identifiers (machine ID or hostname hash) — not user-configured | `machineid.ProtectedID("gsd-node")` gives stable HMAC-SHA256 of OS machine ID; fallback to `hostname + sha256` |
| NODE-02 | Config via `.env`: `SERVER_URL`, `SERVER_TOKEN`, `HEARTBEAT_INTERVAL_SECS` | Extend existing `internal/config/config.go` `Config` struct; add NodeConfig sub-struct or inline fields |
</phase_requirements>

## Summary

Phase 10 creates the stable contract that all downstream phases (11-14) build against. It produces two artifacts: `internal/protocol/messages.go` with all wire types and tests, plus the extended config supporting the three new env vars and hardware-derived node ID.

The Go standard library's `encoding/json` package handles all serialization needs without any external dependency. The `json.RawMessage` field in the envelope struct is the standard Go pattern for polymorphic message dispatching — it defers payload decoding until the `type` field is inspected, which is exactly what a WebSocket dispatcher needs. This pattern is used in production Go WebSocket systems and is well-documented in the standard library.

For node identity, `github.com/denisbrodbeck/machineid` is the standard Go cross-platform library (reads `/var/lib/dbus/machine-id` on Linux, `IOPlatformUUID` on macOS, `MachineGuid` registry on Windows). Its `ProtectedID(appID)` function produces a stable HMAC-SHA256 keyed by the raw machine ID — safe to transmit to a server without exposing the raw OS UUID. A hostname-based fallback (`sha256(hostname)[:16]`) handles edge cases (containers without machine-id, CI environments).

**Primary recommendation:** Create `internal/protocol/messages.go` with all structs using `json.RawMessage` for the envelope payload; add `NodeConfig` fields inline to `internal/config/config.go`; use `denisbrodbeck/machineid` with hostname fallback for NODE-01.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `encoding/json` (stdlib) | Go 1.24 built-in | JSON marshal/unmarshal, `json.RawMessage` for envelope | Zero deps, exactly right for this use case |
| `github.com/denisbrodbeck/machineid` | v1.0.1 | Cross-platform hardware-derived machine ID | Reads OS-native UUID/GUID without admin rights; tested on Linux/macOS/Windows; `ProtectedID` HMAC is safe to transmit |
| `github.com/joho/godotenv` | v1.5.1 | Already in go.mod; load `.env` for new fields | Already in use in `config.go` — no new dep needed |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `crypto/sha256` + `fmt` (stdlib) | built-in | Hostname fallback for node ID | Fallback when `machineid` fails (container, CI) |
| `os/exec` (stdlib) | built-in | Hostname via `os.Hostname()` | Already used in `config.go` — same package |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `denisbrodbeck/machineid` | `github.com/satori/go.uuid` random UUID + persist | Random UUID is not stable across reinstalls; hardware-derived is preferable per NODE-01 |
| `json.RawMessage` envelope | `interface{}` with type switch | `json.RawMessage` avoids double-decode overhead and is the canonical Go stdlib pattern |
| Extending `internal/config/config.go` | New `internal/nodeconfig/` package | Single config load path is simpler; splitting adds boilerplate with no benefit at this scale |

**Installation:**
```bash
go get github.com/denisbrodbeck/machineid@v1.0.1
```

**Version verification:** `denisbrodbeck/machineid` v1.0.1 is the latest release (published 2019-02-27); the module is stable with no breaking changes. All other dependencies are stdlib.

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── protocol/
│   ├── messages.go        # All wire types: Envelope + inbound + outbound structs
│   └── messages_test.go   # Round-trip marshal/unmarshal table tests
├── config/
│   ├── config.go          # Extended with NodeConfig fields (SERVER_URL, SERVER_TOKEN, HEARTBEAT_INTERVAL_SECS)
│   ├── config_test.go     # Extended with new field tests (existing file)
│   └── node_id.go         # DeriveNodeID() — machineid with hostname fallback
```

### Pattern 1: JSON Envelope with `json.RawMessage` Payload

**What:** A two-field outer struct (`type` + `id`) plus a `payload json.RawMessage` that holds the message-type-specific data verbatim until dispatch.

**When to use:** Any protocol where one channel carries N message types and the receiver must branch on `type` before decoding the payload.

**Example:**
```go
// Source: encoding/json stdlib docs — standard two-stage unmarshal
package protocol

import "encoding/json"

// Envelope is the outer frame for every WebSocket message in both directions.
// The Payload field is held as raw JSON until the dispatcher inspects Type.
type Envelope struct {
    Type    string          `json:"type"`
    ID      string          `json:"id"`
    Payload json.RawMessage `json:"payload,omitempty"`
}

// Decode unmarshals the payload into dst. Call after inspecting Type.
func (e *Envelope) Decode(dst any) error {
    return json.Unmarshal(e.Payload, dst)
}

// Encode builds an Envelope from a typed payload. The caller sets Type and ID.
func Encode(msgType, msgID string, payload any) (Envelope, error) {
    raw, err := json.Marshal(payload)
    if err != nil {
        return Envelope{}, err
    }
    return Envelope{Type: msgType, ID: msgID, Payload: raw}, nil
}
```

### Pattern 2: Inbound Message Structs (server-to-node)

**What:** Concrete structs for each command the server sends; registered by `type` string.

```go
// Source: REQUIREMENTS.md PROTO-01 through PROTO-05, INST requirements

// ExecuteCmd is the payload for type="execute" — server requests Claude invocation.
type ExecuteCmd struct {
    InstanceID string `json:"instance_id"`
    Project    string `json:"project"`
    WorkDir    string `json:"work_dir"`
    Prompt     string `json:"prompt"`
    SessionID  string `json:"session_id,omitempty"` // empty = new session
}

// KillCmd is the payload for type="kill" — server requests instance termination.
type KillCmd struct {
    InstanceID string `json:"instance_id"`
}

// StatusRequest is the payload for type="status_request" — server queries running instances.
type StatusRequest struct {
    // intentionally empty — type field is sufficient discriminator
}
```

### Pattern 3: Outbound Message Structs (node-to-server)

**What:** Concrete structs for every event the node emits.

```go
// NodeRegister is the payload for type="node_register".
// RunningInstances provides a snapshot so server can reconcile state after reconnect.
type NodeRegister struct {
    NodeID           string            `json:"node_id"`
    Platform         string            `json:"platform"`          // "linux", "darwin", "windows"
    Version          string            `json:"version"`           // semver string
    Projects         []string          `json:"projects"`
    RunningInstances []InstanceSummary `json:"running_instances"` // PROTO-01 reconnect safety
}

// InstanceSummary is one entry in the RunningInstances snapshot.
type InstanceSummary struct {
    InstanceID string `json:"instance_id"`
    Project    string `json:"project"`
    SessionID  string `json:"session_id,omitempty"`
}

// StreamEvent is the payload for type="stream_event" — one Claude NDJSON chunk.
type StreamEvent struct {
    InstanceID string `json:"instance_id"`
    Data       string `json:"data"` // raw NDJSON line from Claude CLI
}

// InstanceStarted is the payload for type="instance_started".
type InstanceStarted struct {
    InstanceID string `json:"instance_id"`
    Project    string `json:"project"`
    SessionID  string `json:"session_id,omitempty"`
}

// InstanceFinished is the payload for type="instance_finished".
type InstanceFinished struct {
    InstanceID string `json:"instance_id"`
    ExitCode   int    `json:"exit_code"`
}

// InstanceError is the payload for type="instance_error".
type InstanceError struct {
    InstanceID string `json:"instance_id"`
    Error      string `json:"error"`
}
```

### Pattern 4: Node ID Derivation

**What:** Stable, transmittable identifier derived from hardware without user configuration.

```go
// Source: pkg.go.dev/github.com/denisbrodbeck/machineid
package config

import (
    "crypto/sha256"
    "fmt"
    "os"

    "github.com/denisbrodbeck/machineid"
)

// DeriveNodeID returns a stable, transmittable node identifier.
// Strategy:
//   1. machineid.ProtectedID("gsd-node") — HMAC-SHA256 of OS machine UUID keyed by app name.
//   2. Hostname fallback — sha256(hostname)[:16 hex chars] if machineid fails.
func DeriveNodeID() string {
    id, err := machineid.ProtectedID("gsd-node")
    if err == nil && id != "" {
        return id
    }
    // Fallback: hostname hash
    host, _ := os.Hostname()
    sum := sha256.Sum256([]byte(host))
    return fmt.Sprintf("%x", sum[:8]) // 16 hex chars, stable per machine
}
```

### Pattern 5: Config Extension

**What:** Add node-specific fields to the existing `Config` struct; keep a single `Load()` call.

```go
// In internal/config/config.go — extend Config struct:

// ServerURL is the WebSocket server URL (wss://...) the node connects to.
// Required when running as a node (not the Telegram bot).
ServerURL string

// ServerToken is the bearer token for server authentication.
// Required when running as a node.
ServerToken string

// HeartbeatIntervalSecs is the WebSocket ping interval in seconds (default: 30).
HeartbeatIntervalSecs int

// NodeID is the hardware-derived node identifier (set during Load, not from env).
NodeID string
```

New env var parsing follows the identical pattern already used for `RATE_LIMIT_REQUESTS` — `os.Getenv` + `strconv.Atoi` with a documented default.

### Anti-Patterns to Avoid
- **Storing raw OS machine UUID in NodeID:** `machineid.ID()` returns the raw platform UUID which is considered confidential. Always use `ProtectedID()` for transmitted IDs.
- **`interface{}` payload in Envelope:** Causes double-encode problems and loses type safety. Use `json.RawMessage`.
- **Separate config package for node:** Adds indirection with no benefit — extend the existing `Config` struct.
- **`omitempty` on `running_instances`:** Should always be present as an array (empty array `[]`, not `null`) so the server can distinguish "no instances" from "not reported". Use `[]InstanceSummary` not `*[]InstanceSummary`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Cross-platform machine UUID | Custom `/etc/machine-id` reader + registry reader + ioreg parser | `denisbrodbeck/machineid` | Handles 4 OSes, no admin rights, tested since 2019 |
| HMAC node ID | Custom crypto/hmac application | `machineid.ProtectedID(appID)` | Already does HMAC-SHA256 keyed by machine ID |
| JSON dispatch on `type` | `map[string]interface{}` + reflection | `json.RawMessage` + type switch | Standard Go pattern, no reflection, no allocation waste |

**Key insight:** This phase is nearly all standard library. The only external dep is `machineid` for NODE-01; everything else is `encoding/json`, `os`, `crypto/sha256`. The struct definitions are the deliverable — keep them simple and correct.

## Common Pitfalls

### Pitfall 1: `null` vs `[]` for RunningInstances on Empty State
**What goes wrong:** `json.Marshal` encodes a nil Go slice as JSON `null`, not `[]`. The server receives `"running_instances": null` and may error or treat it differently than `"running_instances": []`.
**Why it happens:** Go nil slice marshals to `null`; only an allocated empty slice marshals to `[]`.
**How to avoid:** Initialize `RunningInstances` as `make([]InstanceSummary, 0)` in the constructor or when building the register frame.
**Warning signs:** Round-trip test passes but server integration fails; JSON diff shows `null` vs `[]`.

### Pitfall 2: `machineid` Fails in Containers / CI
**What goes wrong:** `machineid.ID()` reads `/var/lib/dbus/machine-id` which may be absent in minimal Docker images or CI runners; function returns error.
**Why it happens:** Containerized Linux often omits dbus machine-id.
**How to avoid:** The hostname-fallback in `DeriveNodeID()` handles this. The fallback `sha256(hostname)[:16]` is stable within a given container name but will differ across recreations — acceptable for a CI-only scenario.
**Warning signs:** `DeriveNodeID()` test fails in CI but passes locally.

### Pitfall 3: Duplicate `ID` Field Meaning
**What goes wrong:** The `Envelope.ID` field is a message correlation ID (set by sender, echoed in ACK), not the node ID or instance ID. Naming confusion causes downstream bugs in Phase 11/13.
**Why it happens:** Multiple "ID" concepts exist in the protocol.
**How to avoid:** Keep naming precise: `Envelope.ID` = message correlation ID (UUID per message), `NodeRegister.NodeID` = hardware-derived node identity, `ExecuteCmd.InstanceID` = UUID for a Claude subprocess lifetime.

### Pitfall 4: Config `Load()` Requires Telegram Vars
**What goes wrong:** The existing `Load()` returns an error if `TELEGRAM_BOT_TOKEN` or `TELEGRAM_ALLOWED_USERS` are missing. In Phase 12 (cleanup), these fields are removed — but even before cleanup, the node binary should not require Telegram config.
**Why it happens:** `Load()` hard-errors on missing Telegram vars (intentional for the bot use case).
**How to avoid:** Either: (a) make Telegram vars soft-required with an `isTelegramMode` flag, or (b) create a separate `LoadNodeConfig()` function that only validates `SERVER_URL` and `SERVER_TOKEN`. Research recommendation: (b) is cleaner and avoids mutating the existing bot-serving code path before Phase 12.

### Pitfall 5: Test Isolation for `DeriveNodeID()`
**What goes wrong:** `machineid.ProtectedID` reads real OS state — tests are non-deterministic and may fail in CI containers where the machine-id file is absent.
**Why it happens:** Real hardware calls in unit tests.
**How to avoid:** Test that `DeriveNodeID()` returns a non-empty string and is stable across two calls (call twice, compare). Do not assert a specific value. Document the CI fallback behavior in the test comment.

## Code Examples

Verified patterns from official sources:

### Round-Trip Marshal/Unmarshal Test (table-driven, standard Go)
```go
// Source: Go testing conventions — encoding/json standard library
func TestEnvelopeRoundTrip(t *testing.T) {
    tests := []struct {
        name    string
        msgType string
        payload any
    }{
        {
            name:    "node_register",
            msgType: "node_register",
            payload: NodeRegister{
                NodeID:           "abc123",
                Platform:         "linux",
                Version:          "1.2.0",
                Projects:         []string{"/home/user/proj"},
                RunningInstances: make([]InstanceSummary, 0),
            },
        },
        {
            name:    "execute",
            msgType: "execute",
            payload: ExecuteCmd{
                InstanceID: "inst-001",
                Project:    "myproject",
                WorkDir:    "/home/user/proj",
                Prompt:     "fix the bug",
            },
        },
        // ... one case per message type
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            env, err := Encode(tc.msgType, "msg-1", tc.payload)
            if err != nil {
                t.Fatalf("Encode: %v", err)
            }

            // Re-marshal the whole envelope and unmarshal back
            data, _ := json.Marshal(env)
            var got Envelope
            if err := json.Unmarshal(data, &got); err != nil {
                t.Fatalf("Unmarshal envelope: %v", err)
            }
            if got.Type != tc.msgType {
                t.Errorf("Type = %q, want %q", got.Type, tc.msgType)
            }
        })
    }
}
```

### Config Test Extension Pattern (matches existing `config_test.go` style)
```go
func TestLoadNodeConfig(t *testing.T) {
    t.Setenv("SERVER_URL", "wss://example.com/ws")
    t.Setenv("SERVER_TOKEN", "secret-token")
    t.Setenv("HEARTBEAT_INTERVAL_SECS", "30")

    cfg, err := LoadNodeConfig()
    if err != nil {
        t.Fatalf("LoadNodeConfig() error: %v", err)
    }
    if cfg.ServerURL != "wss://example.com/ws" {
        t.Errorf("ServerURL = %q, want wss://example.com/ws", cfg.ServerURL)
    }
    if cfg.HeartbeatIntervalSecs != 30 {
        t.Errorf("HeartbeatIntervalSecs = %d, want 30", cfg.HeartbeatIntervalSecs)
    }
    if cfg.NodeID == "" {
        t.Error("NodeID must be non-empty — hardware derivation failed without fallback")
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| gorilla/websocket concurrent write safety via mutex | coder/websocket + single writer goroutine | 2022 (gorilla archived) | coder/websocket panics on concurrent writes by design; single writer goroutine is the correct architecture |
| Random UUID node ID stored in a file | Hardware-derived `machineid.ProtectedID` | v1.2 design decision | Node survives reinstall with same ID; server can track node identity across reconnects |

**Deprecated/outdated:**
- `gorilla/websocket`: Archived 2022; panics on concurrent writes. Project uses `coder/websocket` (confirmed in STATE.md). Do not reference gorilla patterns.

## Open Questions

1. **Server authentication handshake first-frame format**
   - What we know: `node_register` is the first frame after connect; `SERVER_TOKEN` goes in an HTTP header during the WebSocket upgrade handshake (standard pattern) or in the register payload
   - What's unclear: Whether the server expects token in `Authorization: Bearer` header at upgrade time vs. in the `node_register.token` field vs. both
   - Recommendation: For Phase 10, define a `Token string` field in `NodeRegister` as the primary mechanism. The HTTP header approach requires no protocol struct changes. Both can coexist. Phase 11 (connection) will resolve this operationally — noted as a Phase 11 blocker in STATE.md.

2. **`version` field value in `NodeRegister`**
   - What we know: Version string should be a semver like `"1.2.0"`
   - What's unclear: Whether to hardcode the version string in the protocol package or read from a build-time variable
   - Recommendation: Define a `const Version = "1.2.0"` in the protocol package for Phase 10. Build-time injection (via `-ldflags`) is Phase 12+ scope.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go standard `testing` package (Go 1.24) |
| Config file | None — `go test ./...` discovers tests automatically |
| Quick run command | `go test ./internal/protocol/... ./internal/config/...` |
| Full suite command | `go test ./...` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PROTO-01 | `NodeRegister` struct round-trips marshal/unmarshal with all fields including `running_instances` | unit | `go test ./internal/protocol/... -run TestNodeRegister` | Wave 0 |
| PROTO-02 | `Envelope` with each message type marshals and unmarshals with correct `type` field | unit | `go test ./internal/protocol/... -run TestEnvelopeRoundTrip` | Wave 0 |
| NODE-01 | `DeriveNodeID()` returns non-empty stable string; fallback works when machineid fails | unit | `go test ./internal/config/... -run TestDeriveNodeID` | Wave 0 |
| NODE-02 | `LoadNodeConfig()` parses `SERVER_URL`, `SERVER_TOKEN`, `HEARTBEAT_INTERVAL_SECS`; NodeID populated | unit | `go test ./internal/config/... -run TestLoadNodeConfig` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/protocol/... ./internal/config/...`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/protocol/messages_test.go` — covers PROTO-01, PROTO-02 (new file, new package)
- [ ] `internal/config/node_id.go` — source file for `DeriveNodeID()` (new file)
- [ ] `internal/config/node_id_test.go` — covers NODE-01 (new test file)
- [ ] `internal/config/config.go` extension — covers NODE-02 (extend existing file; `config_test.go` gets new test functions)
- [ ] `go get github.com/denisbrodbeck/machineid@v1.0.1` — add dependency to go.mod

## Sources

### Primary (HIGH confidence)
- `encoding/json` stdlib — `json.RawMessage` two-stage unmarshal pattern; verified against Go 1.24 docs
- `pkg.go.dev/github.com/denisbrodbeck/machineid` — API: `ID()`, `ProtectedID(appID)`, cross-platform support matrix
- `pkg.go.dev/github.com/coder/websocket` v1.8.14 — `wsjson` package confirmed; single-writer correctness requirement confirmed
- Existing codebase: `internal/config/config.go`, `internal/config/config_test.go`, `internal/claude/events.go`, `internal/session/persist.go` — patterns directly observed

### Secondary (MEDIUM confidence)
- WebSearch: "Go machine-id hardware identifier library 2025" — confirmed `denisbrodbeck/machineid` as most referenced library; cross-verified with pkg.go.dev
- WebSearch: "Go json.RawMessage type discriminator envelope unmarshal pattern" — confirmed two-stage unmarshal is standard Go WebSocket protocol pattern

### Tertiary (LOW confidence)
- None. All findings backed by official docs or direct code inspection.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — stdlib + one well-known library; both verified against official pkg.go.dev
- Architecture: HIGH — patterns derived from existing codebase conventions (events.go, persist.go, config_test.go style) and stdlib docs
- Pitfalls: HIGH — null-vs-empty-slice, container machine-id absence, and config required-field issues are all concrete observable behaviors in Go

**Research date:** 2026-03-20
**Valid until:** 2026-09-20 (stable stdlib patterns; `machineid` v1.0.1 has been stable since 2019)
