# Architecture

An operative is a long-running agent that consists of the following components:

* A container sandbox that stays running for the duration of the operative's life
* A configurable model to generate responses
* A single rolling window stream of message entries.

Operatives are implemented in Go with python sandboxes running in containers. The Go code is responsible for managing the sandbox, the stream, and the model. The python code is responsible for executing code in the sandbox and calling back to the Go code to prompt the model via a bidirectional gRPC interface.

## Container Sandbox

The container sandbox is a long-running container that is used to execute code and short processes. It is started when the operative is created and is stopped when the operative is destroyed. A sandbox monitor is responsible for starting and stopping the sandbox. If the monitor notices the sandbox is not running, it should restart it and append a message to the stream indicating that the sandbox was restarted, and what that means for the operative (i.e. that ipython variables are no longer available, and that running processes from before may have been terminated).

### Sandbox Tools

The sandbox provides the following tools to the operative:

* `run_ipython_cell(code: str) -> str` - Runs a single cell of ipython code and returns the output.
  * NOTE: Has a built-in `prompt_model(prompt: str) -> str` function that can be used to prompt a model to inspect a large file and return a summary.
  * Can be used to accomplish all tasks that would otherwise require specific tools.
  * Examples:
    * Calling LLMs to summarize large files
    * Browsing the web
    * Reading and writing files
    * Running math
    * Importing libraries

### Sandbox Persistence

* The sandbox is persistent for the duration of the operative's life under happy path circumstances.
* [FUTURE] The sandbox might support mounting NFS filesystems to allow for persistent storage of files across sandbox restarts or across operatives.
* [FUTURE] The sandbox will support checkpointing and restoring the sandbox state to allow for recovery from crashes. Those crashes will be reported to the operative via a message entry.

**Refactor NOTE:** Most of the logic in the session pkg can be simplified. The key functionality here is appending messages, and triggering compaction.

### Sandbox Server Implementation

* A long running python gRPC program that is started as the main process in the container.
* It exposes the following methods:
  * `RunCell(code: str) -> str` - Runs a single cell of ipython code and returns the output. Called from the operative's control loop.
  * `PromptModel(prompt: str) -> str` - Prompts a model to inspect a large file and return a summary. Called from the ipython code executed by the operative in the sandbox.

### Sandbox Manager Implementation

* The sandbox manager should be a Go interface.
* The sandbox manager should list all known operatives using the operative store interface, it should compare this list to the list of running containers (using label selectors).
* There should be no separate data store for sandboxes. The sandbox manager should implement functions to lookup sandbox state and that should directly call into the underlying runtime for the container.
* Implementation 1: Docker (will require a polling loop to ensure it restarts crashed containers).
* [FUTURE] Implementation 2: Kubernetes Pods - implementation via controller-runtime libraries.

## Stream

The stream is a collection of message entries, user messages, assistant messages, tool calls, and tool results. This is an append-only list of message entries.

### Stream Persistence

A stream store interface should be defined. The default implementation should be sqlite, but it should be possible to implement other backends (e.g. postgres).

### Stream Compaction

As the stream grows, it is compacted by removing chunks of older entries and replacing them with a summary entry. This is done when the session reaches a configurable threshold of entries (default: 60% of max context window tokens). This threshold can be configured per operative. Each model object should contain what the associated max context window size is.

Compaction should be accomplished by the stream manager calling the model to generate a summary of the older entries. The summary should be a single message entry with the role of assistant. Compaction should never occur in the middle of a user message or tool call and tool result. The model should be prompted with these instructions, and it should determine the right place to split the stream to perform the compaction. It should return a tool call with the summary and the id/index of the entry that should be compacted to. Validation should be run to ensure that the compaction did not occur in the middle of a user message or tool call and tool result.

## Operative Controller

* The operative controller is the main control loop for the operative. It is responsible for:
  * Fetching the session context
  * Deciding what action to take next
  * Calling the model
  * Executing tools
  * Compacting the stream

## Operative Persistence

A operative store interface should be defined. The default implementation should be sqlite, but it should be possible to implement other backends in the future (e.g. postgres). This store should contain the operative's instructions, model, and other configuration.

### Operative Instructions

Operatives should be able to update their own instructions. This should be done via a tool call (`update_instructions(instructions: str) -> str`). The instructions should be a text entry that is stored in the operative store. The instructions should be used to generate the system prompt for the model. Instructions should be stored as two separate the admin-set instructions and the operative-set instructions. The operative-set instructions should be appended to the admin-set instructions when generating the system prompt. The admin-set instructions should be immutable to the operative.

## Notes

Operatives can store and search notes. Notes are stored in a separate store from the stream. Notes are not compacted. They are text entries that can be searched by the operative. There should be a Go interface for this store. The default implementation should be sqlite, but it should be possible to implement other backends in the future (e.g. postgres).

The following functions should be available to the operative:

* `store_note(title: str, content: str) -> str` - Stores a note and returns the id of the note.
* `vector_search_notes(query: str) -> List[NoteRef]` - Searches for notes that are semantically similar to the query and returns the results. The results should be the note titles and ids.
* `keyword_search_notes(query: str) -> List[NoteRef]` - Searches for notes that contain the query as a keyword and returns the results. The results should the note titles and ids.
* `get_note(id: str) -> Note` - Returns the note with the given id.
* `delete_note(id: str) -> str` - Deletes the note with the given id.

## UI

A web UI should be provided that allows users to interact with operatives.

* This web UI should be able to list all operatives and their sandbox status.
* The web UI should not have a separate page for sessions, instead an operative page should show the stream for the current operative, along with details about that operative (instructions, model, etc.). You should be able to edit those details from that page.
* The operative page should allow the admin to change the admin-set and operative-set instructions for the operative.
* The web UI should allow you to add a new message to the stream to chat with the operative.
* The web UI should allow you to add a new note to the operative, and view and search existing notes.
