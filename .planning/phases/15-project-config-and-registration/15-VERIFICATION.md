---
phase: 15-project-config-and-registration
verified: 2026-03-25T08:00:00Z
status: passed
score: 4/4 must-haves verified
re_verification: false
---

# Phase 15: Project Config and Registration Verification Report

**Phase Goal:** NodeRegister sends real project list from config so the server knows what projects the node can handle
**Verified:** 2026-03-25T08:00:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #  | Truth                                                                                 | Status     | Evidence                                                          |
|----|---------------------------------------------------------------------------------------|------------|-------------------------------------------------------------------|
| 1  | NodeConfig has a Projects []string field populated from PROJECTS env var              | VERIFIED   | `node_config.go` line 29, parsing block lines 67-74              |
| 2  | Empty or unset PROJECTS env var produces non-nil empty slice ([]string{}), not nil   | VERIFIED   | `cfg.Projects = []string{}` at line 67, TestLoadNodeConfigProjectsEmpty passes |
| 3  | NodeRegister frame sent on connect includes the configured project list from config   | VERIFIED   | `register.go` line 21: `Projects: m.cfg.Projects`                |
| 4  | Projects round-trip through registration (config -> NodeRegister -> JSON wire format) | VERIFIED   | TestRegisterOnConnect passes: sets 2 projects, asserts both fields on received frame |

**Score:** 4/4 truths verified

---

### Required Artifacts

| Artifact                                    | Expected                                                           | Status   | Details                                                                              |
|---------------------------------------------|--------------------------------------------------------------------|----------|--------------------------------------------------------------------------------------|
| `internal/config/node_config.go`            | Projects field on NodeConfig, PROJECTS env var parsing             | VERIFIED | `Projects []string` at line 29; full parsing block at lines 66-74                   |
| `internal/config/node_config_test.go`       | Tests for PROJECTS parsing: populated, empty, whitespace-trimmed   | VERIFIED | TestLoadNodeConfigProjects, TestLoadNodeConfigProjectsEmpty, TestLoadNodeConfigProjectsSingleItem all present and passing |
| `internal/connection/register.go`           | sendRegister uses m.cfg.Projects instead of hard-coded []string{}  | VERIFIED | Line 21: `Projects: m.cfg.Projects` — no hard-coded `[]string{}` remains             |
| `internal/connection/manager_test.go`       | TestRegisterOnConnect asserts Projects field, newTestConfig includes Projects | VERIFIED | newTestConfig includes `Projects: []string{}` at line 28; TestRegisterOnConnect at lines 185-244 asserts both NodeID and Projects |

---

### Key Link Verification

| From                                  | To                                  | Via                                      | Status   | Details                                                    |
|---------------------------------------|-------------------------------------|------------------------------------------|----------|------------------------------------------------------------|
| `internal/config/node_config.go`      | `internal/connection/register.go`   | `m.cfg.Projects` accessed in sendRegister | VERIFIED | `register.go` line 21 reads `m.cfg.Projects` directly      |
| `internal/config/node_config.go`      | `.env.example`                      | PROJECTS env var documented for operators | VERIFIED | `.env.example` line 19: `# PROJECTS=my-project,another-project` |

---

### Data-Flow Trace (Level 4)

This phase produces no rendering artifacts — it wires configuration through to a wire-format frame. The critical data-flow is: env var -> NodeConfig.Projects -> sendRegister -> JSON wire frame.

| Stage                         | Mechanism                              | Produces Real Data | Status    |
|-------------------------------|----------------------------------------|--------------------|-----------|
| PROJECTS env var parsing      | `os.Getenv("PROJECTS")` + comma split  | Yes                | FLOWING   |
| NodeConfig.Projects field     | Populated by LoadNodeConfig()          | Yes                | FLOWING   |
| sendRegister assignment       | `Projects: m.cfg.Projects`             | Yes                | FLOWING   |
| JSON wire frame               | Verified by TestRegisterOnConnect      | Yes                | FLOWING   |

---

### Behavioral Spot-Checks

| Behavior                                              | Command                                                                             | Result                                    | Status |
|-------------------------------------------------------|-------------------------------------------------------------------------------------|-------------------------------------------|--------|
| Config Projects tests pass                            | `go test ./internal/config/... -run TestLoadNodeConfigProjects -v`                  | 3/3 tests PASS                            | PASS   |
| Registration Projects round-trip test passes          | `go test ./internal/connection/... -run TestRegisterOnConnect -v -count=1`          | PASS                                      | PASS   |
| Full config and connection suites pass                | `go test ./internal/config/... ./internal/connection/... -v -count=1`              | All tests pass (config OK, connection OK) | PASS   |
| Task 1 commit exists                                  | `git show --stat b7d41ea`                                                           | Commit found, +77 lines across 2 files    | PASS   |
| Task 2 commit exists                                  | `git show --stat 991e177`                                                           | Commit found, net +29/-100 across 4 files | PASS   |

---

### Requirements Coverage

| Requirement | Source Plan | Description                                                                           | Status    | Evidence                                                                                           |
|-------------|-------------|---------------------------------------------------------------------------------------|-----------|----------------------------------------------------------------------------------------------------|
| PROTO-01    | 15-01-PLAN  | Node sends registration frame on connect: node_id (hardware-derived), platform, project list, version | SATISFIED | `register.go` sends NodeRegister with NodeID, Platform, Version, and now `m.cfg.Projects` (not hard-coded empty). TestRegisterOnConnect asserts the full payload. |
| NODE-02     | 15-01-PLAN  | Config via `.env`: SERVER_URL, SERVER_TOKEN, HEARTBEAT_INTERVAL_SECS                  | SATISFIED | `node_config.go` parses all three required vars plus new PROJECTS. `.env.example` documents all fields for operators. Existing tests (TestLoadNodeConfig, TestLoadNodeConfigDefaults) continue to pass. |

No orphaned requirements found — both IDs declared in plan frontmatter and both confirmed present in REQUIREMENTS.md mapped to Phase 15.

---

### Anti-Patterns Found

No anti-patterns found in the five modified files.

- `register.go`: No remaining `[]string{}` hard-code; `m.cfg.Projects` is a real config value.
- `node_config.go`: `cfg.Projects = []string{}` is a safe non-nil default before conditional append — not a stub; gets populated from env var.
- `node_config_test.go`: All three new test functions contain real assertions, not placeholders.
- `manager_test.go`: TestRegisterOnConnect upgraded from `chan string` to `chan protocol.NodeRegister` and asserts both NodeID and Projects fields.
- `.env.example`: No Telegram vars remain; PROJECTS documented as commented example.

---

### Human Verification Required

None. All goal behaviors are verifiable programmatically via Go tests and grep. No visual, real-time, or external-service behavior is involved.

---

### Gaps Summary

No gaps. All four observable truths are verified, all four required artifacts exist and are substantive and wired, both key links are confirmed, both requirements are satisfied, and the full test suites pass.

---

_Verified: 2026-03-25T08:00:00Z_
_Verifier: Claude (gsd-verifier)_
