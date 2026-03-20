---
phase: 3
slug: media-handlers-and-windows-service
status: compliant
nyquist_compliant: true
wave_0_complete: true
created: 2026-03-20
last_audited: 2026-03-20
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib) |
| **Config file** | none — go test ./... discovers all *_test.go |
| **Quick run command** | `go test ./internal/handlers/... -run TestMedia -v -count=1` |
| **Full suite command** | `go test ./... -count=1` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/handlers/... -count=1`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 03-01-01 | 01 | 1 | MEDIA-01 | unit | `go test ./internal/handlers/... -run TestHandleVoice` | ✅ | ✅ green |
| 03-01-02 | 01 | 1 | MEDIA-01 | unit | `go test ./internal/handlers/... -run TestHandleVoice_NoAPIKey` | ✅ | ✅ green |
| 03-02-01 | 02 | 1 | MEDIA-02 | unit | `go test ./internal/handlers/... -run TestHandlePhoto_Single` | ✅ | ✅ green |
| 03-02-02 | 02 | 1 | MEDIA-03 | unit | `go test ./internal/handlers/... -run TestMediaGroupBuffer` | ✅ | ✅ green |
| 03-03-01 | 03 | 1 | MEDIA-04 | unit | `go test ./internal/handlers/... -run TestHandleDocument_PDF` | ✅ | ✅ green |
| 03-03-02 | 03 | 1 | MEDIA-04 | unit | `go test ./internal/handlers/... -run TestHandleDocument_PDFError` | ✅ | ✅ green |
| 03-03-03 | 03 | 1 | MEDIA-05 | unit | `go test ./internal/handlers/... -run TestHandleDocument_Text` | ✅ | ✅ green |
| 03-03-04 | 03 | 1 | MEDIA-05 | unit | `go test ./internal/handlers/... -run TestHandleDocument_Unsupported` | ✅ | ✅ green |
| 03-03-05 | 03 | 1 | MEDIA-05 | unit | `go test ./internal/handlers/... -run TestHandleDocument_TooBig` | ✅ | ✅ green |
| 03-04-01 | 04 | 2 | DEPLOY-02 | manual | review docs | N/A | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/handlers/voice_test.go` — stubs for MEDIA-01 (transcription + no-API-key)
- [x] `internal/handlers/photo_test.go` — stubs for MEDIA-02, MEDIA-03 (single + album)
- [x] `internal/handlers/document_test.go` — stubs for MEDIA-04, MEDIA-05 (PDF, text, unsupported, size)
- [x] `internal/handlers/media_group_test.go` — stubs for MediaGroupBuffer timer logic

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| NSSM service installs and starts at boot | DEPLOY-02 | Requires Windows reboot + NSSM binary | Install service, reboot, verify bot running |
| Voice message transcription end-to-end | MEDIA-01 | Requires real Telegram voice + OpenAI key | Send voice, verify transcript + Claude response |
| Photo album rendering in Claude | MEDIA-03 | Requires real Telegram album | Send 3 photos, verify Claude addresses all |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 10s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved (2026-03-20)
