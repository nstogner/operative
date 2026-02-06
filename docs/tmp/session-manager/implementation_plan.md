# Implementation Plan - Go SessionManager

## Goal
Implement a robust, thread-safe `SessionManager` in Go, adhering to the approved [Design Document](docs/session-manager/design_doc.md).

## Proposed Changes

### 1. Project Initialization
- Initialize Go module: `go mod init github.com/your-org/coding-agent/session` (or similar, verifying path).

### 2. Core Data Structures (`pkg/session/types.go`)
- Define `Entry` struct using "Tagged Union" pattern (pointers to specific Entry structs).
- Define specific Entry structs (`MessageEntry`, `ModelChangeEntry`, etc.).
- Define `Block`, `Content`, and specific block types (`TextBlock`, `ImageBlock`, etc.).
- Ensure `snake_case` JSON tags throughout.

### 3. Session Logic (`pkg/session/session.go`)
- Implement `Session` struct with `sync.RWMutex`.
- Implement `Append` method (thread-safe, updates `LeafID`, recursive parent pointers).
- Implement `Branch` method (creates offshoots).
- Implement `Context` building (traversal from leaf to root).

### 4. Manager Logic (`pkg/session/manager.go`)
- Implement `SessionManager` struct.
- File system operations (Load, Save - append only).
- Concurrency handling for multiple sessions.

### 5. Utilities (`pkg/session/utils.go`)
- Compaction helpers.
- Hash generation (if needed for IDs).

## Verification Plan

### Automated Tests
- **Unit Tests**:
    - `TestSession_Append`: Verify chain integrity and map updates.
    - `TestSession_Branch`: Verify branching logic and parent references.
    - `TestSerialization`: Verify JSON output matches the "Nested Structure" requirement.
- **Integration Tests**:
    - Simulate full interaction loops (User -> Assistant -> Tool -> Assistant).
    - Verify file persistence (write to disk, read back, ensure identity).

### Manual Verification
- Run the `examples/` generation code to produce `simple_session.jsonl` output and diff against the design doc examples.
