# Server Backend Specification

> Derived from the working GSD node implementation (v1.2.0).
> For wire format details (Envelope structure, JSON encoding, message ID format), see `docs/protocol-spec.md`.

## 1. Overview

The server is the central management plane for GSD nodes. It accepts inbound WebSocket connections from nodes, dispatches commands (execute, kill, status_request), receives streaming output and lifecycle events, and tracks per-node and per-instance state.

Key architectural properties:

- **Nodes connect outbound to the server** -- the server never connects to nodes. This makes nodes NAT-friendly with no firewall changes required on the node side.
- **Communication:** WebSocket over TLS, JSON text frames. Every frame is an `Envelope` with `type`, `id`, and `payload` fields (see `docs/protocol-spec.md`).
- **Server responsibilities:** accept node connections, authenticate via Bearer token, dispatch commands to nodes, receive streaming output, track node/instance state, handle reconnection and state reconciliation.

## 2. WebSocket Endpoint

### Endpoint

Single WebSocket upgrade endpoint:

```
wss://server.example.com/ws/node
```

### Authentication

During the HTTP upgrade handshake, validate the `Authorization` header:

```
Authorization: Bearer {token}
```

- **Missing header:** return HTTP `401 Unauthorized`, do not upgrade.
- **Invalid token:** return HTTP `403 Forbidden`, do not upgrade.
- **Token validation mechanism** is the server's choice (static shared secret, JWT, database lookup). The node sends the token configured in its `SERVER_TOKEN` environment variable.

### First Frame Expectation

After a successful WebSocket upgrade, the server MUST expect a `node_register` message as the **first frame** from the node.

- If the first frame is not a valid `node_register`, close the connection with an error.
- The node sends `node_register` synchronously before starting its reader/writer/heartbeat goroutines, so it is guaranteed to be the first frame.

### Connection Behavior

The server must handle these node-side behaviors:

| Behavior | Detail |
|----------|--------|
| **Automatic reconnection** | Nodes reconnect with exponential backoff (500ms to 30s) after any connection drop. The server must accept re-registration gracefully -- the same `node_id` will appear in a new `node_register`. |
| **Heartbeat pings** | Nodes send WebSocket-level pings at a configurable interval (default: every 30 seconds). The server must respond with pongs (standard WebSocket protocol behavior). |
| **Ping timeout** | Nodes consider a connection dead if no pong is received within 3x the heartbeat interval (default: 90 seconds). The node will close and reconnect. |
| **Clean disconnect** | Before closing, the node sends a `node_disconnect` frame with `reason: "shutdown"`, then sends a WebSocket close frame with status 1000 (Normal Closure). |
| **Reconnect with state** | On reconnect, `node_register` includes a `running_instances` array -- a snapshot of all Claude CLI instances still running on the node. This enables state reconciliation (see Section 6). |

## 3. Data Models

### Node Model

The server must track the following per connected node:

| Field | Type | Description |
|-------|------|-------------|
| `node_id` | string | Stable hardware-derived identifier (primary key). Uses `machineid.ProtectedID` with hostname SHA-256 fallback. The same physical node reconnecting will always have the same `node_id`. |
| `platform` | string | OS identifier from Go's `runtime.GOOS` (e.g., `"windows"`, `"linux"`, `"darwin"`). |
| `version` | string | Node software version. Currently `"1.2.0"` (matches `protocol.Version`). |
| `projects` | string[] | Project names the node has configured. May be empty `[]` (never null). |
| `connected_at` | timestamp | When the current WebSocket session started. |
| `last_heartbeat` | timestamp | Last successful ping/pong exchange. |
| `status` | enum | `connected`, `disconnected`, `stale`. |

### Instance Model

The server must track the following per running Claude CLI instance:

| Field | Type | Description |
|-------|------|-------------|
| `instance_id` | string | Server-assigned UUID, unique across all nodes. The server generates this when dispatching an `execute` command. |
| `node_id` | string | Which node is running this instance. |
| `project` | string | Project name the instance is running under. |
| `session_id` | string (optional) | Claude CLI session ID for resume capability. Initially populated from `instance_started`; updated (superseded) by `instance_finished` which carries the authoritative final session ID. The server should persist the `instance_finished` value for future `--resume` use. |
| `status` | enum | `pending` (execute sent, no ACK yet), `running` (ACK or `instance_started` received), `finished`, `errored`. |
| `started_at` | timestamp | When `instance_started` was received from the node. |
| `finished_at` | timestamp (optional) | When a terminal event (`instance_finished` or `instance_error`) was received. |

### Relationships

- One node has zero or more instances.
- An instance belongs to exactly one node.
- On node reconnect: reconcile `running_instances` from `node_register` with the server's tracked instances (see Section 6).

## 4. Command Dispatch

The server sends commands to nodes as Envelope-wrapped JSON text frames. Each command uses a unique message ID -- the server should generate its own IDs in the same format as the node (random hex string, 32 characters / 16 bytes).

### execute -- Start a Claude CLI Instance

The server generates an `instance_id` (UUID) before sending this command.

**Envelope:**
```json
{
  "type": "execute",
  "id": "<server-generated-msg-id>",
  "payload": {
    "instance_id": "<server-generated-uuid>",
    "project": "my-project",
    "work_dir": "/path/to/project",
    "prompt": "User message to send to Claude",
    "session_id": ""
  }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `instance_id` | Yes | Server-generated UUID for this execution. |
| `project` | Yes | Project name configured on the node. |
| `work_dir` | Yes | Working directory for the Claude CLI subprocess. |
| `prompt` | Yes | The user message to send to Claude. |
| `session_id` | No | To resume a previous Claude session. Empty string or omitted starts a new session. |

**Expected response flow:**

```
Server sends:  execute
Node sends:    ack                    (immediate, correlated by envelope ID)
Node sends:    instance_started       (subprocess launched, includes session_id)
Node sends:    stream_event           (zero or more NDJSON output chunks)
Node sends:    stream_event
Node sends:    ...
Node sends:    instance_finished      (clean exit, includes exit_code)
               OR instance_error      (error, includes error message)
```

- The `ack` uses the same envelope `id` as the `execute` command for correlation.
- If the node rate-limits the request: an immediate `instance_error` with error `"rate limited"` is sent. No `ack` or `instance_started` will follow.
- Exactly one terminal event (`instance_finished` or `instance_error`) is guaranteed per instance.

### kill -- Terminate a Running Instance

**Envelope:**
```json
{
  "type": "kill",
  "id": "<server-generated-msg-id>",
  "payload": {
    "instance_id": "<instance-id-to-kill>"
  }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `instance_id` | Yes | The instance to terminate. |

- The node cancels the instance's context, which terminates the Claude CLI subprocess.
- A terminal event (`instance_finished` or `instance_error`) will follow -- the server should **wait for it** rather than immediately marking the instance as killed.
- If the instance is not found on the node, the kill is silently ignored (logged on the node side).

### status_request -- Query Current Node State

**Envelope:**
```json
{
  "type": "status_request",
  "id": "<server-generated-msg-id>",
  "payload": {}
}
```

- Empty payload (no fields).
- The node responds with a `node_register` message containing its current `running_instances`.
- The response uses the `status_request`'s envelope `id` for correlation.

## 5. Inbound Event Handling

Events the server receives from nodes:

| Event | Trigger | Server Action |
|-------|---------|---------------|
| `node_register` | Connect, reconnect, status response | Upsert node record; reconcile instances (see Section 6). |
| `ack` | After `execute` received | Mark instance as `running`; correlate via envelope `id` (matches the `execute` command's `id`). |
| `stream_event` | Claude CLI output | Forward to frontend/store, keyed by `instance_id`. Contains NDJSON chunks in `data` field. |
| `instance_started` | Subprocess launched | Update instance status to `running`; capture `session_id` for resume capability. |
| `instance_finished` | Clean exit | Mark instance `finished`; record `exit_code` (0 = clean, -1 = signal-killed, positive = CLI error); capture `session_id` for resume capability; set `finished_at` timestamp. |
| `instance_error` | Error or rate limit | Mark instance `errored`; record `error` message; set `finished_at` timestamp. |
| `node_disconnect` | Clean shutdown | Mark node `disconnected`; expect WebSocket close to follow. |

### Terminal Event Guarantee

Each instance produces **exactly one** terminal event (`instance_finished` or `instance_error`). The node uses a `sync.Once` gate internally to prevent duplicates even in race conditions (e.g., kill command arriving simultaneously with natural exit). The server can rely on this: **no deduplication of terminal events is needed**.

### Envelope Payload Structures

**ack:**
```json
{ "instance_id": "..." }
```

**stream_event:**
```json
{ "instance_id": "...", "data": "<NDJSON line from Claude CLI>" }
```

**instance_started:**
```json
{ "instance_id": "...", "project": "...", "session_id": "..." }
```

**instance_finished:**
```json
{ "instance_id": "...", "exit_code": 0, "session_id": "..." }
```

**instance_error:**
```json
{ "instance_id": "...", "error": "human-readable error description" }
```

**node_disconnect:**
```json
{ "reason": "shutdown" }
```

## 6. State Reconciliation on Reconnect

When a node reconnects (sends `node_register` with `running_instances`):

1. **Compare** `running_instances` from the new `node_register` with the server's tracked instances for that `node_id`.

2. **Instances in server but NOT in node's list:** Mark as `errored` / lost. The node likely crashed during execution, and the instances were terminated without sending terminal events.

3. **Instances in node's list but NOT in server:** Add to tracking as `running`. This handles the case where the server restarted while a node was actively running instances.

4. **Instances in both:** Update `session_id` if changed; confirm `running` status.

This reconciliation is critical for reliability: neither node nor server crashes should cause permanent state divergence.

### Reconciliation Flow

```
node_register arrives with running_instances = [A, B, C]
server has tracked instances for this node  = [A, B, D]

Result:
  A: exists in both    -> update session_id if changed, confirm running
  B: exists in both    -> update session_id if changed, confirm running
  C: node has, server doesn't -> add to tracking as running (server restart case)
  D: server has, node doesn't -> mark as errored/lost (node crash case)
```

## 7. Node Health Monitoring

### Heartbeat-Based Health

- Nodes send WebSocket-level pings every 30 seconds (configurable on the node via `HEARTBEAT_INTERVAL_SECS`).
- The server must track a `last_heartbeat` timestamp per node.
- **Stale threshold:** If no ping is received for >90 seconds (3x the default interval), consider the node stale.
- **Connection drop:** If the WebSocket connection drops, mark the node as `disconnected` immediately.
- **Stale/disconnected nodes:** Mark all their tracked instances as `errored` / lost.

### Clean Disconnect

- The node sends a `node_disconnect` frame (with `reason: "shutdown"`) before closing the WebSocket.
- The server should mark the node as `disconnected` (not crashed).
- All instances for that node should have already sent terminal events. The node drains running instances before disconnecting -- the shutdown sequence is: stop dispatcher, wait for instances (up to 10 seconds), then close the WebSocket connection.

### Recommended Server Health States

| State | Condition |
|-------|-----------|
| `connected` | Active WebSocket connection, pings arriving. |
| `stale` | WebSocket open but no ping received for >90s. |
| `disconnected` | WebSocket closed (either clean disconnect or connection drop). |

## 8. OpenAI Whisper Integration Point

### Purpose

Server-side voice-to-text transcription for the web frontend. Users speak into the web UI; audio is sent to the server; the server transcribes via OpenAI Whisper API; the transcribed text becomes the `prompt` field in an `execute` command dispatched to the target node.

### Integration Architecture

```
[Browser] --audio--> [Server REST endpoint] --transcribe--> [OpenAI Whisper API]
                                |
                                v
                     prompt = transcribed text
                                |
                                v
              [Server dispatches execute command to node via WebSocket]
```

1. **Frontend** captures audio using the browser MediaRecorder API.
2. **Frontend** sends the audio file to the server via a **REST endpoint** (NOT via WebSocket -- binary audio does not belong in the JSON WebSocket protocol).
3. **Server** calls the OpenAI Whisper API:
   - Endpoint: `POST https://api.openai.com/v1/audio/transcriptions`
   - Model: `whisper-1`
   - Supported formats: mp3, mp4, mpeg, mpga, m4a, wav, webm
   - Max file size: 25 MB
   - Returns: transcribed text
4. **Server** uses the transcribed text as the `prompt` in an `execute` command dispatched to the target node.

This is entirely server-side -- **nodes have no knowledge of voice input**.

### Required Server Configuration

| Variable | Required | Description |
|----------|----------|-------------|
| `OPENAI_API_KEY` | Yes (for voice) | API key for OpenAI Whisper authentication. |
| Language hint | No | Optional language hint for improved accuracy. |
| Response format | No | Optional response format preference (default: text). |

### Suggested Endpoints

- `POST /api/transcribe` -- accepts audio file, returns transcribed text as JSON.
- Or integrate directly: `POST /api/execute` accepts either a text prompt or an audio file, transcribing before dispatch.

## 9. Security Considerations

### Token Rotation

The server should support rotating `SERVER_TOKEN` without dropping connected nodes. Recommended approach: accept both old and new tokens during a grace period, then revoke the old token.

### Node Identity

`node_id` is hardware-derived (via `machineid.ProtectedID` with hostname SHA-256 fallback). The server should:
- Track a `first_seen` timestamp per `node_id`.
- Alert on new `node_id` values (potential unauthorized node).
- Reject connections from revoked `node_id` values if a deny list is maintained.

### Rate Limiting

- Nodes enforce per-project rate limits locally (using token bucket rate limiters).
- The server may additionally enforce global rate limits across all nodes (e.g., max concurrent instances per user/project).

### Command Validation

Before dispatching an `execute` command, the server should validate:
- The `project` exists on the target node (check against the `projects` list from `node_register`).
- The `work_dir` is a valid path for the target project.
- The target node is `connected` (not `stale` or `disconnected`).

### Audit Trail

Log all commands dispatched and events received with:
- `node_id` -- which node was involved
- `instance_id` -- which instance (when applicable)
- Timestamp
- Command type / event type
- Error details (for `instance_error` events)

## 10. Deployment Topology

```
[Web Frontend] --REST/WS--> [Server] <--WSS-- [Node 1]
                                      <--WSS-- [Node 2]
                                      <--WSS-- [Node N]
                              |
                              v
                        [OpenAI Whisper API]
```

### Key Properties

- **Nodes initiate connections outbound** -- NAT-friendly, no firewall changes needed on the node side.
- **Server listens on a single port** for both frontend (REST/WebSocket) and node WebSocket connections (different endpoints, e.g., `/ws/node` for nodes, `/ws/frontend` for the web UI).
- **Server is the only component** that talks to the OpenAI Whisper API.
- **Nodes are stateless from the server's perspective** -- all persistent state lives on the server. Nodes can be restarted at any time; they will reconnect and send a fresh `node_register` with their current state.

### Connection Summary

| Direction | Protocol | Authentication |
|-----------|----------|---------------|
| Node -> Server | WSS (WebSocket over TLS) | `Authorization: Bearer {token}` header during HTTP upgrade |
| Frontend -> Server | HTTPS + WSS | Server's choice (session cookies, JWT, etc.) |
| Server -> OpenAI | HTTPS | `Authorization: Bearer {OPENAI_API_KEY}` |
