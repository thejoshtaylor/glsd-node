---
phase: 10-protocol-definitions-and-config
plan: "02"
subsystem: config
tags: [go, machineid, hardware-id, node-config, websocket, environment]

requires:
  - phase: 10-01
    provides: Protocol message types and Envelope struct (same phase)

provides:
  - DeriveNodeID() function with machineid primary and hostname-sha256 fallback
  - NodeConfig struct with ServerURL, ServerToken, HeartbeatIntervalSecs, NodeID
  - LoadNodeConfig() that parses WebSocket env vars without requiring Telegram config
  - machineid v1.0.1 dependency in go.mod

affects:
  - phase-11 (WebSocket connection lifecycle — uses NodeConfig)
  - phase-12 (session management — uses NodeID for node identity)
  - phase-13 (server backend spec — NodeConfig defines what node sends at connect)

tech-stack:
  added: [github.com/denisbrodbeck/machineid v1.0.1]
  patterns:
    - TDD RED-GREEN cycle for all new Go code
    - godotenv.Load() at top of config functions (ignore error if .env absent)
    - strconv.Atoi with named error message for int env vars
    - DeriveNodeID called inside LoadNodeConfig (not from env)

key-files:
  created:
    - internal/config/node_id.go
    - internal/config/node_id_test.go
    - internal/config/node_config.go
    - internal/config/node_config_test.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "machineid.ProtectedID used as primary ID source (HMAC-SHA256 of OS machine UUID, app-scoped)"
  - "Hostname sha256[:8 bytes] as fallback covers containers and CI where machine UUID is unavailable"
  - "NodeConfig is fully separate from Config — no Telegram env vars required"
  - "HeartbeatIntervalSecs defaults to 30 seconds to match common WebSocket ping conventions"

patterns-established:
  - "Hardware ID derivation: machineid primary, sha256(hostname) fallback — stable across boots"
  - "Separate config structs per subsystem — avoid coupling unrelated services"

requirements-completed: [NODE-01, NODE-02]

duration: 12min
completed: 2026-03-20
---

# Phase 10 Plan 02: Node ID and Config Summary

**Hardware-derived NodeID via machineid with hostname-sha256 fallback, plus standalone NodeConfig parsing SERVER_URL/SERVER_TOKEN without any Telegram dependency**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-03-20T23:10:00Z
- **Completed:** 2026-03-20T23:22:00Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- DeriveNodeID returns stable HMAC-SHA256 machine ID with hostname fallback for containers/CI
- NodeConfig struct cleanly separates WebSocket configuration from Telegram Config
- LoadNodeConfig parses three env vars, auto-populates NodeID, requires no Telegram setup
- 8 new tests added; all 19 config package tests pass (existing tests unmodified)

## Task Commits

Each task was committed atomically:

1. **Task 1: Add machineid dependency and create DeriveNodeID** - `3757a1c` (feat)
2. **Task 2: Create NodeConfig struct and LoadNodeConfig function** - `4c9a759` (feat)

**Plan metadata:** (docs commit to follow)

_Note: Both tasks used TDD RED-GREEN cycle._

## Files Created/Modified

- `internal/config/node_id.go` - DeriveNodeID() with machineid primary and hostname fallback
- `internal/config/node_id_test.go` - TestDeriveNodeID and TestDeriveNodeIDStable
- `internal/config/node_config.go` - NodeConfig struct and LoadNodeConfig() function
- `internal/config/node_config_test.go` - 6 tests covering all parsing paths and error cases
- `go.mod` - Added github.com/denisbrodbeck/machineid v1.0.1
- `go.sum` - Updated with machineid checksums

## Decisions Made

- Used `machineid.ProtectedID("gsd-node")` as primary source — HMAC scoped to app name prevents cross-app ID collisions
- Hostname sha256 truncated to 8 bytes (16 hex chars) for fallback — sufficient uniqueness at reasonable length
- HeartbeatIntervalSecs defaults to 30 (not 60) — matches common WebSocket ping-pong conventions
- NodeConfig intentionally does not call `godotenv.Load()` in a way that errors if .env absent — `_ =` pattern from existing config.go

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

Go binary was not in bash PATH (Windows environment). Required adding `/c/Program Files/Go/bin` to PATH for each shell command. This is an environment-level issue, not a code issue — no code changes needed.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- NodeConfig and DeriveNodeID ready for Phase 11 WebSocket connection lifecycle
- NodeID provides stable per-machine identity for server registration
- Concern from STATE.md still applies: `coder/websocket` read deadline API differs from gorilla — verify method names before writing connection lifecycle code in Phase 11

---
*Phase: 10-protocol-definitions-and-config*
*Completed: 2026-03-20*
