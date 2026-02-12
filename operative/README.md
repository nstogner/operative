# Operative

A long-running AI agent system with persistent sandbox environments, a rolling message stream, and searchable notes.

## Architecture

```
cmd/operative/main.go         Entrypoint — wires everything together
pkg/
  domain/                      Core types: Operative, StreamEntry, Note, Model
  store/                       Store interfaces (OperativeStore, StreamStore, NoteStore)
    sqlite/                    SQLite implementation (WAL mode, auto-migration)
  model/                       Provider interface (Name, List, Stream)
    gemini/                    Google Gemini implementation
  sandbox/                     Manager interface (Run, RunCell, Status, Close)
    docker/                    Docker container implementation + gRPC sandbox
  controller/                  Event-driven control loop + tool dispatch + compaction
  server/                      HTTP API + WebSocket + SPA static serving
web/                           React + TypeScript + Vite + Tailwind + shadcn/ui
```

**Control flow:** Stream event → Controller step → Call model or execute tool → Append result → Check compaction.

**Sandbox lifecycle:** `main.go` launches `sbMgr.Run(ctx, store)` in a goroutine on startup. The Run loop polls `ListIDs()` every 10s, starts containers for known operatives, and stops orphaned ones. `RunCell()` assumes the container is already running and returns an error if not.

**System instructions:** Built from three sources: (1) static environment/tools description, (2) admin-set instructions, (3) operative self-set instructions.

**Tools:** `run_ipython_cell`, `update_instructions`, `store_note`, `keyword_search_notes`, `vector_search_notes`, `get_note`, `delete_note`.

**Data:** SQLite with three tables (`operatives`, `stream_entries`, `notes`). Stream compaction replaces older entries with a model-generated summary when token usage exceeds a configurable threshold.

## Requirements

- Go 1.21+
- Node.js 18+
- `GEMINI_API_KEY` environment variable
- Docker (for sandbox containers)
- CGO enabled (`CGO_ENABLED=1`, required by `go-sqlite3`)

## Commands

| Command | Description |
|---------|-------------|
| `make dev` | Run Go backend + Vite dev server (HMR) |
| `make build` | Build frontend + single Go binary → `bin/operative` |
| `make test` | Run unit tests |
| `make test-integration` | Run integration tests (needs API key + Docker) |
| `make test-e2e` | Run Playwright E2E tests |
| `make build-sandbox` | Build the `sandbox-python:latest` Docker image |
| `make install-deps` | Install Go + npm dependencies |

## Quick Start

```bash
cp .env.example .env  # Add your GEMINI_API_KEY
make build-sandbox    # Build sandbox Docker image
make dev
# Frontend: http://localhost:5173
# Backend:  http://localhost:8080
```

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operatives` | List operatives |
| POST | `/api/operatives` | Create operative |
| GET/PUT/DELETE | `/api/operatives/:id` | CRUD operative |
| GET | `/api/operatives/:id/stream` | Get stream entries |
| GET/POST | `/api/operatives/:id/notes` | List / create notes |
| GET | `/api/operatives/:id/notes/keyword-search?q=` | Keyword search |
| GET | `/api/operatives/:id/sandbox/status` | Sandbox status |
| GET | `/api/models` | List available models |
| WS | `/api/operatives/:id/chat` | Real-time chat |
