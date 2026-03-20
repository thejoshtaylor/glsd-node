---
phase: 03-media-handlers-and-windows-service
plan: "03"
subsystem: handlers/document
tags: [media, document, pdf, text-files, tdd, go]
dependency_graph:
  requires:
    - internal/handlers/helpers.go (downloadToTemp, extractPDF, isTextFile, maxFileSize, maxTextChars)
    - internal/handlers/media_group.go (MediaGroupBuffer)
    - internal/config/config.go (Config.PdfToTextPath)
  provides:
    - internal/handlers/document.go (HandleDocument, classifyDocument, buildDocumentPrompt, truncateText, supportedExtensionsList)
  affects:
    - src/index.ts registration (handler wiring in main)
tech_stack:
  added: []
  patterns:
    - HandleText enqueue pattern (mapping check, GetOrCreate, worker, StreamingState, enqueue)
    - MediaGroupBuffer for document album batching with 1s timeout
    - classifyDocument routing (pdf/text/unsupported)
key_files:
  created:
    - internal/handlers/document.go
    - internal/handlers/document_test.go
    - internal/handlers/photo_stub.go
  modified: []
decisions:
  - "truncateText uses byte-length not rune-length for simplicity (consistent with maxTextChars constant)"
  - "Document album snippets stored as pre-formatted prompt strings in MediaGroupBuffer paths array (avoids extra data structure)"
  - "photo_stub.go created to unblock parallel compilation while Plan 03-02 develops photo.go"
metrics:
  duration_minutes: 8
  completed: "2026-03-20T03:58:02Z"
  tasks_completed: 1
  tasks_total: 1
  test_count: 4
  files_created: 3
  files_modified: 0
---

# Phase 03 Plan 03: Document Handler Summary

Document handler with PDF extraction via pdftotext CLI and text/code file reading, supporting albums via MediaGroupBuffer.

## What Was Built

### HandleDocument (`internal/handlers/document.go`)

Core document processing handler following the established HandleText pattern:

1. **classifyDocument** - Routes files by extension: `.pdf` -> "pdf", text extensions -> "text", everything else -> "unsupported"
2. **buildDocumentPrompt** - Constructs `[Document: filename]\ncontent\n\ncaption` prompt format
3. **truncateText** - Caps extracted content at `maxTextChars` (100K) with "..." ellipsis
4. **supportedExtensionsList** - Sorted comma-separated list for the unsupported-type error message
5. **HandleDocument** - Full handler: mapping check, 10MB size guard, classify, download, extract (PDF or text), album buffering or single-doc enqueue
6. **sendDocToSession** - Shared enqueue helper for single docs and album callbacks
7. **makeDocAlbumProcessor** - MediaGroupBuffer callback that joins document snippets with `---` separators

### Flow

```
Document message -> size check (10MB) -> classify (pdf/text/unsupported)
  -> download -> extract content
  -> album? -> MediaGroupBuffer (1s timeout) -> join snippets -> enqueue
  -> single? -> buildDocumentPrompt -> enqueue to Claude session
```

### Tests (`internal/handlers/document_test.go`)

- **TestClassifyDocument** - 12 cases: .pdf, .PDF, .py, .ts, .json, .md, .sh, .csv, .png, .exe, .zip, .jpg
- **TestBuildDocumentPrompt** - With and without caption
- **TestTruncateText** - Short (no-op), long (truncated), exact, empty
- **TestSupportedExtensionsList** - Contains expected extensions, sorted order verified

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Created photo_stub.go for parallel compilation**
- **Found during:** Task 1 test execution
- **Issue:** Plan 03-02 left `photo_test.go` referencing `buildSinglePhotoPrompt`, `buildAlbumPrompt`, and `HandlePhoto` which don't exist yet (photo.go not committed)
- **Fix:** Created `internal/handlers/photo_stub.go` with minimal implementations matching test expectations
- **Files created:** internal/handlers/photo_stub.go
- **Commit:** 3db9d8d

## Commits

| Hash | Message |
|------|---------|
| 3db9d8d | feat(03-03): implement HandleDocument with PDF extraction and text file reading |

## Self-Check: PASSED

- internal/handlers/document.go: FOUND
- internal/handlers/document_test.go: FOUND
- internal/handlers/photo_stub.go: superseded by Plan 03-02's photo.go (in commit 3db9d8d history, replaced on disk by real photo.go)
- Commit 3db9d8d: FOUND
- `go build ./...`: PASSED
- `go test` (document tests): 4/4 PASSED
