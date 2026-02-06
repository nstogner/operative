# Gemini Instructions

## Project Overview

This is an event-driven CLI coding agent powered by Google Gemini.

### Core Architecture

The system follows a reactive, event-driven pattern:
1.  **Session Loop**: State is immutable and stored as a sequence of events (messages) in JSONL files.
2.  **Orchestration**: A background `Runner` listens for file/session updates. When a session changes, the Runner evaluates the state and triggers the next necessary action (calling the LLM or executing a tool).

### Packages

-   **`cmd/cli`**: The entry point. Provides an interactive REPL.
    -   Initializes the `GeminiModel`, `ToolRegistry`, and `Runner`.
    -   Handles user input by appending `User` messages to the session.
    -   Listens for updates to print `Assistant` responses.

-   **`pkg/session`**: Message handling.
    -   **`Manager`**: Handles creating, loading, and persisting sessions. Implemented via `jsonl`.
    -   **`Session`**: Represents a thread of conversation. Exposes methods to append messages.
    -   **Events**: The Manager publishes events (Session IDs) whenever persistence occurs.

-   **`pkg/runner`**: The brain.
    -   **`Runner`**: Subscribes to the `session.Manager`. On event, loads the session and calls `RunStep`.
    -   **`RunStep`**: Stateless logic function.
        -   If last message is `User` or `ToolResult` -> Calls LLM.
        -   If last message is `Assistant` with Tool Calls -> Executes Tools.
        -   Appends results back to Session (triggering another event).

-   **`pkg/models`**: LLM Abstractions.
    -   `ModelProvider`: Interface for listing models and streaming responses.
    -   `pkg/models/gemini`: Implementation using `google-generative-ai-go`.

-   **`pkg/tools`**: Capabilities.
    -   **`Registry`**: dictionaries of available tools.
    -   **Tools**: `ListFiles`, `ReadFile`, `WriteFile`.

### Usage

```bash
export GEMINI_API_KEY="your-key"
go run cmd/cli/main.go
```
1.  Select a model from the list.
2.  Chat or ask to perform file operations (e.g., "List files in this folder").