# Phase 7: Phase 3 Verification and Nyquist Compliance - Research

**Researched:** 2026-03-20
**Domain:** GSD verification and Nyquist compliance artifacts — no new code, documentation-and-audit-only phase
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None — all implementation choices are at Claude's discretion.

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase. This is a verification and compliance exercise: run observable checks against existing code, produce VERIFICATION.md and VALIDATION.md artifacts, and update tracking files.

### Deferred Ideas (OUT OF SCOPE)
None
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| MEDIA-01 | User can send voice messages; bot transcribes via OpenAI Whisper and processes as text | voice.go exists, HandleVoice implemented, voice_test.go exists — needs observable truth verification |
| MEDIA-02 | User can send photos; bot forwards to Claude for visual analysis | photo.go exists, HandlePhoto implemented, photo_test.go exists — needs observable truth verification |
| MEDIA-03 | Bot buffers photo albums (media groups) with a timeout before sending as a batch | media_group.go and MediaGroupBuffer implemented, media_group_test.go exists — needs observable truth verification |
| MEDIA-04 | User can send PDF documents; bot extracts text via pdftotext and sends to Claude | document.go exists, extractPDF in helpers.go — needs observable truth verification |
| MEDIA-05 | User can send text/code files as documents; bot reads content and sends to Claude | document.go classifyDocument routing — needs observable truth verification |
| DEPLOY-02 | Bot installs as a Windows Service (runs at boot, no terminal window) | docs/windows-service.md exists — manual-only, needs observable truth documentation |
</phase_requirements>

---

## Summary

Phase 7 is a pure verification-and-compliance phase. All six requirements it covers (MEDIA-01 through MEDIA-05, DEPLOY-02) were implemented in Phase 3 but never formally verified — no 03-VERIFICATION.md was written, and both 03-VALIDATION.md and 04-VALIDATION.md remain in `nyquist_compliant: false` draft state.

The implementation is confirmed complete: `voice.go`, `photo.go`, `document.go`, `helpers.go`, `media_group.go` all exist and pass tests. The bot dispatcher wires all three media handlers correctly in `internal/bot/handlers.go`. The NSSM documentation exists at `docs/windows-service.md`. The full test suite (`go test ./...`) passes all 9 packages.

This phase requires three deliverables: (1) write `03-VERIFICATION.md` with observable truths verified against the existing code for all six requirements, (2) update ROADMAP.md to mark Phase 3 Complete, and (3) update both `03-VALIDATION.md` and `04-VALIDATION.md` frontmatter to `nyquist_compliant: true` with a sign-off on all checklist items.

**Primary recommendation:** This is a documentation task. Write VERIFICATION.md by inspecting actual source line-by-line (same method as Phase 2 and 4 verifications), then run the test suite one final time to confirm green before updating the frontmatter.

---

## Standard Stack

This phase introduces no new libraries. The existing project stack applies:

### Core (already installed)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib testing | 1.26.1 | Test runner | Standard Go toolchain |
| gotgbot/v2 | current | Telegram API client | Project choice since Phase 1 |
| golang.org/x/time/rate | current | Rate limiter | Project choice since Phase 2 |

**Installation:** None needed. All dependencies already in go.mod.

---

## Architecture Patterns

### Verification File Pattern (from Phases 2 and 4)

The established VERIFICATION.md format has five sections:

```
---
phase: {slug}
verified: {RFC3339 timestamp}
status: passed | human_needed | gaps_found
score: N/N must-haves verified
re_verification: false
---

# Phase N: {Name} — Verification Report

## Goal Achievement

### Observable Truths
| # | Truth | Status | Evidence |
|---|-------|--------|----------|
...

### Required Artifacts
| Artifact | Expected | Status | Details |
...

### Key Link Verification
| From | To | Via | Status | Details |
...

### Requirements Coverage
| Requirement | Source Plan | Description | Status | Evidence |
...

### Anti-Patterns Found
...

### Human Verification Required
...

## Build and Test Results
...

## Gaps Summary
...
```

Status field values:
- `passed` — all automated checks pass, no human items
- `human_needed` — all automated checks pass but some behaviors require live bot
- `gaps_found` — automated gaps remain

Score format: `N/N must-haves verified` where N = count of Observable Truths rows.

### Nyquist Compliance Pattern (from Phase 1 VALIDATION.md)

A VALIDATION.md is `nyquist_compliant: true` when:
1. All tasks have an automated verify command OR a Wave 0 dependency (test file that will contain the test)
2. No 3 consecutive tasks without automated verify
3. Wave 0 covers all MISSING file references
4. No watch-mode flags in any command
5. Feedback latency is under the stated threshold
6. `nyquist_compliant: true` is set in frontmatter

For Phase 3 and Phase 4, the VALIDATION.md files already have complete per-task maps and Wave 0 lists. The only change needed is:
- Update `status: draft` to `status: compliant`
- Update `nyquist_compliant: false` to `nyquist_compliant: true`
- Update `wave_0_complete: false` to `wave_0_complete: true`
- Tick the sign-off checklist

### ROADMAP.md Update Pattern (from ROADMAP.md Phase 3 entry)

Phase 3 line to change:
```
- [ ] **Phase 3: Media Handlers and Windows Service** - Voice, photo, PDF processing and Windows Service deployment
```
Becomes:
```
- [x] **Phase 3: Media Handlers and Windows Service** - Voice, photo, PDF processing and Windows Service deployment (completed 2026-03-20)
```

Also update the progress table row:
```
| 3. Media Handlers and Windows Service | 1/4 | In Progress | - |
```
Becomes:
```
| 3. Media Handlers and Windows Service | 4/4 | Complete   | 2026-03-20 |
```

And update the per-phase Plans section from `- [ ]` to `- [x]` for plans 02, 03, 04.

### REQUIREMENTS.md Traceability Update

Six requirements need status change from `Pending` to `Complete`:

| Requirement | Current | Target |
|-------------|---------|--------|
| MEDIA-01 | `[ ]` + Pending | `[x]` + Complete |
| MEDIA-02 | `[ ]` + Pending | `[x]` + Complete |
| MEDIA-03 | `[ ]` + Pending | `[x]` + Complete |
| MEDIA-04 | `[ ]` + Pending | `[x]` + Complete |
| MEDIA-05 | `[ ]` + Pending | `[x]` + Complete |
| DEPLOY-02 | `[ ]` + Pending | `[x]` + Complete |

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Verifying file existence | Custom filesystem walker | Direct file check with `ls` or `Read` tool | Simpler, faster |
| Verifying function signatures | Regex parser | Read the source file directly | Accurate, no false positives |
| Updating frontmatter | sed/awk scripts | Edit tool with precise replacement | Safer, auditable |
| Updating ROADMAP.md | gsd-tools.cjs `phase complete` | Direct Edit tool | `phase complete` may not handle partial-status rows correctly; Edit is deterministic |

**Key insight:** This is a documentation phase. The primary tool is the Read/Edit tool, not any code generator. The value is in the careful line-by-line inspection that produces accurate evidence strings in the Observable Truths table.

---

## Common Pitfalls

### Pitfall 1: Stale Test Evidence
**What goes wrong:** Citing test counts from SUMMARY.md files (which were written at implementation time) rather than running the test suite at verification time.
**Why it happens:** SUMMARY.md says "67 tests pass" — but subsequent phases may have added or broken tests.
**How to avoid:** Run `go test ./internal/handlers/... -v -count=1` during verification to get the current live count and confirm all tests still pass.
**Warning signs:** Evidence strings that reference commit hashes instead of current line numbers.

### Pitfall 2: Incomplete Plan-Checkbox Updates in ROADMAP.md
**What goes wrong:** Updating the phase-level checkbox and status table but leaving plan-level checkboxes (`- [ ] 03-02-PLAN.md`) still unchecked.
**Why it happens:** The progress table is the obvious target; the per-plan checkbox list is easy to overlook.
**How to avoid:** Update all four plan lines (03-01 through 03-04) plus the phase header and progress table row.

### Pitfall 3: Wrong Status Field in VALIDATION.md
**What goes wrong:** Setting `nyquist_compliant: true` without also updating `status: draft` to `status: compliant` and `wave_0_complete: false` to `wave_0_complete: true`.
**Why it happens:** Phase 1 VALIDATION.md shows all three fields must be updated together.
**How to avoid:** Update all three frontmatter fields in a single Edit operation.

### Pitfall 4: Missing Human Verification Section
**What goes wrong:** VERIFICATION.md omits "Human Verification Required" for DEPLOY-02 and live media tests.
**Why it happens:** The verifier focuses on automated evidence and forgets the manual-only items listed in 03-VALIDATION.md.
**How to avoid:** The 03-VALIDATION.md already lists three manual-only items: NSSM service install, voice end-to-end, and photo album end-to-end. Include all three in the human verification section.

### Pitfall 5: REQUIREMENTS.md Traceability Table Not Updated
**What goes wrong:** VERIFICATION.md and ROADMAP.md are updated but REQUIREMENTS.md traceability table still shows `Pending` for the six requirements.
**Why it happens:** REQUIREMENTS.md has two separate places that need updating: the requirement definition (checkbox) and the traceability table at the bottom.
**How to avoid:** Update both the `- [ ] **MEDIA-01**` definition lines and the traceability table rows for all six requirements.

---

## Code Examples

### Current ROADMAP.md Phase 3 Status (Observed)

The Phase 3 progress table row currently reads:
```
| 3. Media Handlers and Windows Service | 1/4 | In Progress | - |
```
This is inaccurate — all 4 plans completed. The `1/4` reflects only 03-01 being marked complete in the Plans section.

### Current 03-VALIDATION.md Frontmatter (Observed)

```yaml
---
phase: 3
slug: media-handlers-and-windows-service
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-20
---
```

Target after update:
```yaml
---
phase: 3
slug: media-handlers-and-windows-service
status: compliant
nyquist_compliant: true
wave_0_complete: true
created: 2026-03-20
last_audited: 2026-03-20
---
```

### Current 04-VALIDATION.md Frontmatter (Observed)

```yaml
---
phase: 4
slug: callback-handler-integration-fixes
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-19
---
```

Target after update:
```yaml
---
phase: 4
slug: callback-handler-integration-fixes
status: compliant
nyquist_compliant: true
wave_0_complete: true
created: 2026-03-19
last_audited: 2026-03-20
---
```

### Observable Truth Evidence Strings (Pre-verified)

These are the key evidence strings the planner can include in 03-VERIFICATION.md Observable Truths. Each is verified against current source:

| Req | Truth | Evidence location |
|-----|-------|------------------|
| MEDIA-01 | HandleVoice checks OpenAI API key and returns friendly error if absent | `voice.go` lines 64-67: `if cfg.OpenAIAPIKey == ""` guard |
| MEDIA-01 | HandleVoice downloads OGG and transcribes with 60s timeout | `voice.go` lines 83, 97-100: `downloadToTemp + transcribeVoice with context.WithTimeout(60s)` |
| MEDIA-01 | HandleVoice enqueues transcript to Claude session following HandleText pattern | `voice.go` lines 138-201: `store.GetOrCreate`, `wg.Add(1)`, `sess.Worker`, `sess.Enqueue` |
| MEDIA-02 | HandlePhoto selects largest PhotoSize and downloads to temp file | `photo.go`: `photos[len(photos)-1]`, `downloadToTemp` with `.jpg` suffix |
| MEDIA-02 | HandlePhoto builds `[Photo: /path]` prompt for single photos | `buildSinglePhotoPrompt` confirmed by `TestBuildSinglePhotoPrompt` PASS |
| MEDIA-03 | MediaGroupBuffer.Add resets timer per item to extend the batch window | `media_group.go`: `time.AfterFunc` + `sync.Mutex`; `TestMediaGroupBuffer_MultipleItems` PASS |
| MEDIA-03 | Album callback produces `[Photos:\n1. ...\n2. ...]` format | `buildAlbumPrompt` confirmed by `TestBuildAlbumPrompt` PASS |
| MEDIA-04 | extractPDF runs `pdftotext -layout <file> -` and returns partial output on non-zero exit | `helpers.go` extractPDF: partial extraction on non-zero exit; `TestExtractPDF_Success` PASS |
| MEDIA-04 | HandleDocument routes `.pdf` extension to PDF extraction | `classifyDocument` confirmed by `TestClassifyDocument` 12-case test PASS |
| MEDIA-05 | HandleDocument routes text extensions to isTextFile path and reads content | `isTextFile` 17-entry extension map; `TestIsTextFile_Extensions` PASS |
| MEDIA-05 | HandleDocument enforces 10MB size limit and 100K char content truncation | `maxFileSize = 10*1024*1024`, `maxTextChars = 100_000`; `TestConstants_Helpers` PASS |
| DEPLOY-02 | NSSM Windows Service documentation exists at docs/windows-service.md | File confirmed present; 5 sections: Prerequisites, Install, Manage, Env Vars, Troubleshooting |

### Handler Dispatcher Wiring (Verified)

From `internal/bot/handlers.go` lines 38-40:
```go
dispatcher.AddHandler(handlers.NewMessage(message.Voice, b.handleVoice))
dispatcher.AddHandler(handlers.NewMessage(message.Photo, b.handlePhoto))
dispatcher.AddHandler(handlers.NewMessage(message.Document, b.handleDocument))
```

From `internal/bot/handlers.go` lines 56-68:
```go
func (b *Bot) handleVoice(tgBot *gotgbot.Bot, ctx *ext.Context) error {
    return bothandlers.HandleVoice(tgBot, ctx, b.store, b.cfg, b.auditLog, b.persist, b.WaitGroup(), b.mappings, b.globalAPILimiter)
}
// (handlePhoto and handleDocument follow identical pattern)
```

All three media handler wrappers pass the full parameter set including `auditLog`, `persist`, `WaitGroup()`, and `globalAPILimiter` — Phase 6 safety parameters are included.

---

## State of the Art

| Old State | Current State | When Changed | Impact |
|-----------|---------------|--------------|--------|
| 03-VALIDATION.md: draft, nyquist_compliant: false | Will become compliant: true | Phase 7 | Phase 3 Nyquist compliant |
| 04-VALIDATION.md: draft, nyquist_compliant: false | Will become compliant: true | Phase 7 | Phase 4 Nyquist compliant |
| Phase 3 ROADMAP status: In Progress (1/4) | Will become Complete (4/4) | Phase 7 | Accurate milestone tracking |
| 6 requirements PENDING in REQUIREMENTS.md | Will become Complete | Phase 7 | Traceability closure |
| No 03-VERIFICATION.md exists | Will be created | Phase 7 | Closes audit gap |

---

## Open Questions

1. **REQUIREMENTS.md — Phase 7 traceability row**
   - What we know: Traceability table currently maps MEDIA-01..05 and DEPLOY-02 to "Phase 7" with status "Pending"
   - What's unclear: Should the traceability table update say "Phase 3" (where implementation lives) or "Phase 7" (current phase assignment)?
   - Recommendation: Update Phase column from "Phase 7" to "Phase 3" since these requirements were implemented in Phase 3, and set Status to "Complete". Phase 7 is the verification phase, not the implementation phase.

2. **04-VALIDATION.md Wave 0 items**
   - What we know: 04-VALIDATION.md lists Wave 0 as `- [ ] internal/handlers/callback_integration_test.go`
   - What's unclear: The file now exists (confirmed by `ls internal/handlers/`), so Wave 0 is actually complete
   - Recommendation: Update Wave 0 checkbox to `[x]` and set `wave_0_complete: true`.

3. **03-VALIDATION.md Wave 0 items**
   - What we know: 03-VALIDATION.md lists 4 Wave 0 test files as missing (`❌ W0`)
   - What's unclear: All four (`voice_test.go`, `photo_test.go`, `document_test.go`, `media_group_test.go`) now exist (confirmed by `ls internal/handlers/`)
   - Recommendation: Update all four Wave 0 checkboxes to `[x]`, update task map `File Exists` column from `❌ W0` to `✅`, and update statuses from `⬜ pending` to `✅ green`.

---

## Validation Architecture

Nyquist validation is enabled (`workflow.nyquist_validation: true` in config.json).

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib 1.26.1 |
| Config file | none — `go test ./...` discovers all `*_test.go` |
| Quick run command | `"/c/Program Files/Go/bin/go" test ./internal/handlers/... -count=1` |
| Full suite command | `"/c/Program Files/Go/bin/go" test ./... -count=1` |

### Phase Requirements — Test Map

Phase 7 is a documentation-only phase. Its tasks are write/edit operations on `.md` files, not code. The verification method is reading source files and running the existing test suite.

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| MEDIA-01 | Voice handler exists and no-API-key guard works | unit | `go test ./internal/handlers/... -run TestHandleVoice -v` | ✅ voice_test.go |
| MEDIA-02 | Photo handler exists and prompt format correct | unit | `go test ./internal/handlers/... -run TestBuildSinglePhotoPrompt -v` | ✅ photo_test.go |
| MEDIA-03 | MediaGroupBuffer batches and fires after timeout | unit | `go test ./internal/handlers/... -run TestMediaGroupBuffer -v` | ✅ media_group_test.go |
| MEDIA-04 | PDF classification and extraction | unit | `go test ./internal/handlers/... -run TestClassifyDocument -v` | ✅ document_test.go |
| MEDIA-05 | Text file classification and size limits | unit | `go test ./internal/handlers/... -run TestIsTextFile -v` | ✅ helpers_test.go |
| DEPLOY-02 | NSSM docs exist at docs/windows-service.md | manual | file read / review | ✅ docs/windows-service.md |

### Sampling Rate
- **Per task commit:** `"/c/Program Files/Go/bin/go" test ./... -count=1`
- **Per wave merge:** `"/c/Program Files/Go/bin/go" test ./... -count=1`
- **Phase gate:** Full suite green before phase complete

### Wave 0 Gaps
None — existing test infrastructure covers all phase requirements. This phase writes documentation files, not test files.

---

## Sources

### Primary (HIGH confidence)
- Direct source code inspection: `internal/handlers/voice.go`, `photo.go`, `document.go`, `helpers.go`, `media_group.go`, `internal/bot/handlers.go` — all read and verified line-by-line
- Live test run: `go test ./... -count=1` — all 9 packages PASS (confirmed during research)
- Existing VERIFICATION.md templates: `04-VERIFICATION.md` (Phase 4), `02-VERIFICATION.md` (Phase 2) — format confirmed from actual files
- Existing VALIDATION.md templates: `01-VALIDATION.md` (nyquist_compliant: true example), `03-VALIDATION.md`, `04-VALIDATION.md` (targets to update)
- `.planning/ROADMAP.md` — current state confirmed (Phase 3 In Progress, 1/4 plans checked)
- `.planning/REQUIREMENTS.md` — current state confirmed (6 requirements Pending)

### Secondary (MEDIUM confidence)
- Phase 3 SUMMARY.md files (03-01 through 03-04) — narrative descriptions of what was built, cross-checked against source

### Tertiary (LOW confidence)
- None

---

## Metadata

**Confidence breakdown:**
- What files exist and what they contain: HIGH — directly read and verified
- Observable truths for VERIFICATION.md: HIGH — pre-verified against source lines
- ROADMAP.md update targets: HIGH — current state read, required changes clear
- VALIDATION.md update targets: HIGH — current frontmatter read, target state clear
- DEPLOY-02 (NSSM): HIGH for docs existence, MEDIUM for operational correctness (manual-only)

**Research date:** 2026-03-20
**Valid until:** Stable (documentation-only phase; no moving ecosystem targets)
