---
phase: quick
plan: 260324-wld
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/config/node_config.go
  - internal/config/node_config_test.go
  - internal/dispatch/dispatcher_test.go
autonomous: true
must_haves:
  truths:
    - "LoadNodeConfig rejects ws:// URLs with a clear error message"
    - "All test fixtures use wss:// URLs"
    - "Existing tests continue to pass"
  artifacts:
    - path: "internal/config/node_config.go"
      provides: "wss:// validation in LoadNodeConfig"
      contains: "wss://"
    - path: "internal/config/node_config_test.go"
      provides: "Test for ws:// rejection"
      contains: "ws://"
  key_links:
    - from: "internal/config/node_config.go"
      to: "internal/connection/dial.go"
      via: "cfg.ServerURL passed to websocket.Dial"
      pattern: "websocket\\.Dial.*m\\.cfg\\.ServerURL"
---

<objective>
Enforce wss:// (TLS) for all WebSocket connections by adding URL scheme validation to LoadNodeConfig and fixing the one test that uses an unencrypted ws:// URL.

Purpose: Prevent accidental plaintext WebSocket connections that would expose bearer tokens and message payloads.
Output: Validated config rejects ws://, all tests use wss://.
</objective>

<context>
@internal/config/node_config.go
@internal/config/node_config_test.go
@internal/dispatch/dispatcher_test.go
</context>

<tasks>

<task type="auto">
  <name>Task 1: Add wss:// scheme validation to LoadNodeConfig and test ws:// rejection</name>
  <files>internal/config/node_config.go, internal/config/node_config_test.go</files>
  <action>
In `internal/config/node_config.go`, after the existing `SERVER_URL` empty check (line 38-40), add validation that the URL must start with "wss://". Use `strings.HasPrefix` (strings is already available in the test file; add the import to node_config.go). Return error: `fmt.Errorf("SERVER_URL must use wss:// scheme (got %q) — plaintext ws:// is not allowed", cfg.ServerURL)`.

Add `"strings"` to the import block.

In `internal/config/node_config_test.go`, add a new test `TestLoadNodeConfigRejectsInsecureWS`:
- Set SERVER_URL to "ws://example.com/ws", SERVER_TOKEN to "secret"
- Assert LoadNodeConfig returns non-nil error
- Assert error message contains "wss://"
- Assert error message contains "ws://" (the rejected scheme)
  </action>
  <verify>
    <automated>cd /Users/josh/code/glsd-node && go test ./internal/config/ -run TestLoadNodeConfig -v -count=1</automated>
  </verify>
  <done>LoadNodeConfig rejects ws:// URLs with a descriptive error. New test confirms rejection. All existing config tests still pass.</done>
</task>

<task type="auto">
  <name>Task 2: Fix dispatcher test to use wss:// URL</name>
  <files>internal/dispatch/dispatcher_test.go</files>
  <action>
In `internal/dispatch/dispatcher_test.go` line 175, change `"ws://localhost:9999"` to `"wss://localhost:9999"`. This is a test fixture URL (never actually dialed in that test) so the change is safe.
  </action>
  <verify>
    <automated>cd /Users/josh/code/glsd-node && go test ./internal/dispatch/ -v -count=1</automated>
  </verify>
  <done>No remaining ws:// literals in non-test-rejection code. Dispatcher tests pass.</done>
</task>

</tasks>

<verification>
```bash
# Confirm no ws:// literals remain outside the rejection test
cd /Users/josh/code/glsd-node && grep -rn 'ws://' --include='*.go' | grep -v 'node_config_test.go' | grep -v '_test.go.*wss://'
# Should return empty (the only ws:// left is in the rejection test)

# Full test suite
cd /Users/josh/code/glsd-node && go test ./internal/config/ ./internal/dispatch/ ./internal/connection/ -count=1
```
</verification>

<success_criteria>
- LoadNodeConfig returns an error when SERVER_URL uses ws:// scheme
- All WebSocket test fixtures use wss:// URLs
- All existing tests pass without modification (except the dispatcher fixture fix)
- `grep -rn 'ws://' --include='*.go'` shows only the rejection test case
</success_criteria>
