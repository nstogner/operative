# Gemini Instructions

## Project Overview

This is an event-driven coding agent powered by Google Gemini, featuring a **React-based Web Interface**.

### Core Architecture

The system follows a reactive, event-driven pattern:
1.  **Session Loop**: State is immutable and stored as a sequence of events (messages) in JSONL files.
2.  **Orchestration**: A background `Runner` listens for file/session updates. When a session changes, the Runner evaluates the state and triggers the next necessary action (calling the LLM or executing a tool).
3.  **Web Interface**: A React frontend communicates with the Go backend via REST API (for management) and WebSockets (for real-time chat).

### Packages

-   **`cmd/cli`**: The entry point. Starts the HTTP server and Runner.
    -   Initializes components (`Manager`, `Runner`, `Server`).
    -   Serves the Web UI.

-   **`pkg/server`**: HTTP and WebSocket server.
    -   Exposes REST API for Agents and Sessions.
    -   Handles real-time WebSocket connections for chat.
    -   Serves embedded static assets (`web/dist`).

-   **`web`**: The Frontend Application.
    -   Built with **React**, **TypeScript**, **Vite**, **Tailwind CSS**, and **shadcn/ui**.
    -   Located in `web/` directory.

-   **`pkg/session`**: Message handling.
    -   **`Manager`**: Handles creating, loading, and persisting sessions. Implemented via `jsonl`.
    -   **`Session`**: Represents a thread of conversation. Exposes methods to append messages.
    -   **Events**: The Manager publishes events (Session IDs) whenever persistence occurs.

-   **`pkg/runner`**: The brain.
    -   **`Runner`**: Subscribes to the `session.Manager`. On event, loads the session and calls `RunStep`.
    -   **`RunStep`**: Stateless logic function.

-   **`pkg/models`**: LLM Abstractions.
    -   `ModelProvider`: Interface for listing models and streaming responses.
    -   `pkg/models/gemini`: Implementation using `google-generative-ai-go`.

### Usage

**Prerequisites**:
-   Go 1.21+
-   Node.js 18+
-   `GEMINI_API_KEY` environment variable.

#### Development Mode
Run the Go backend and Vite dev server concurrently for Hot Module Replacement (HMR).
```bash
make dev
```
-   Frontend: `http://localhost:5173`
-   Backend: `http://localhost:8080`

You need to re-run `make dev` after you make changes to the Go program.

#### Production Build
Build the frontend and embed it into a single Go binary.
```bash
make build
./bin/gemini
```
-   Access at: `http://localhost:8080`

### Testing

-   **Unit Tests**:
    ```bash
    make test
    ```

-   **Integration Tests**:
    ```bash
    make test-integration
    ```

-   **E2E Tests**:
    ```bash
    make test-e2e
    ```