# Gemini Instructions

## Project Overview

This is an AI agent system called **Operative** — long-running agents with persistent sandbox environments, searchable notes, and a web UI.

### Core Architecture

The system follows an event-driven, reactive pattern:
1. **Stream-based state**: All conversation state is stored as stream entries in SQLite.
2. **Controller loop**: A `Controller` subscribes to stream events. When a new entry appears (user message or tool result), it calls the LLM, executes tools, and checks compaction.
3. **Sandbox manager**: A background `Run()` loop continuously reconciles Docker containers for each operative. Containers host an IPython kernel accessible via gRPC.
4. **Web Interface**: React frontend communicates via REST API and WebSockets.

### Packages

- **`cmd/operative`**: Entrypoint. Initializes store, model provider, sandbox manager, controller, and server.

- **`pkg/domain`**: Core types — `Operative`, `StreamEntry`, `Note`, `Model`, `ToolCall`, `ToolResult`.

- **`pkg/store`**: Store interfaces (`OperativeStore`, `StreamStore`, `NoteStore`).
  - **`pkg/store/sqlite`**: SQLite implementation with WAL mode and auto-migration. Also implements `sandbox.OperativeLister` via `ListIDs()`.

- **`pkg/model`**: `Provider` interface with `Name()`, `List()`, `Stream()`.
  - **`pkg/model/gemini`**: Google Gemini implementation using `google-generative-ai-go`.

- **`pkg/sandbox`**: `Manager` interface with `Run()`, `RunCell()`, `Status()`, `Close()`. Also defines `OperativeLister` and `Delegate` interfaces.
  - **`pkg/sandbox/docker`**: Docker-based implementation. Manages container lifecycle via a reconciliation loop. Communicates with the Python sandbox via gRPC (bidirectional streaming).

- **`pkg/controller`**: The brain. Subscribes to stream events, orchestrates model calls and tool execution, manages compaction. System instructions are built from three sources: static environment description, admin instructions, and operative self-set instructions.

- **`pkg/server`**: HTTP/WebSocket server. REST API for operatives, streams, notes, models. WebSocket endpoint for real-time chat. Serves embedded React frontend.

- **`web/`**: React + TypeScript + Vite + Tailwind + shadcn/ui frontend.

### Usage

**Prerequisites**:
- Go 1.21+
- Node.js 18+
- Docker daemon running
- `GEMINI_API_KEY` environment variable (can be set in `.env`)

#### Development Mode
```bash
make dev
```
- Frontend: `http://localhost:5173`
- Backend: `http://localhost:8080`

You need to re-run `make dev` after you make changes to the Go program.

#### Production Build
```bash
make build
./bin/operative
```
- Access at: `http://localhost:8080`

### Testing

- **Unit Tests**: `make test`
- **Integration Tests**: `make test-integration` (requires `GEMINI_API_KEY` and Docker)
- **E2E Tests**: `make test-e2e`
