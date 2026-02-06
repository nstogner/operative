# Design Doc: Agent Session (Go Orchestration)

## 1. Overview
This document outlines the design for `AgentSession`, an orchestration layer that sits above the `SessionManager`. While `SessionManager` handles the persistence and tree-structured history, `AgentSession` manages the agent's lifecycle, model interactions, thinking levels, auto-compaction, auto-retry, and tool execution.

## 2. Goals & Non-Goals

### Goals
*   **Agent Lifecycle**: Manage initialization, state restoration, and shutdown of an agent.
*   **Orchestrated Prompting**: Handle skill expansion, prompt templates, and streaming interaction (steer/follow-up).
*   **Multi-Model Support**: Manage model selection, API key validation, and model cycling.
*   **Context Management**: Orchestrate manual and automatic compaction based on context usage and overflow errors.
*   **Resilience**: Implement automatic retry logic with exponential backoff for transient provider errors.
*   **Tool Orchestration**: Manage tool registration and execution, including bash execution.
*   **Event Propagation**: Proxy agent events and emit session-specific events (e.g., compaction progress).

### Non-Goals
*   **Persistence Implementation**: Storage logic (JSONL, tree traversal) belongs to `SessionManager`.
*   **Reasoning Logic**: The actual decision-making is handled by the underlying LLM/Agent.
*   **UI Concerns**: This is an orchestration layer, not a UI component.

## 3. Control Flow & Agent Loop

The `AgentSession` implements an autonomous agent loop that continues to execute as long as there are pending tasks (tool calls or follow-up messages).

### 3.1. The Main Loop (`Run`)
The loop operates in three nested layers:

1.  **Outer Loop (Autonomous Session)**:
    *   Continues as long as there are **Follow-up** messages queued.
    *   This allows the agent to perform multi-step autonomous tasks without user intervention.
2.  **Middle Loop (Turn Management)**:
    *   Continues as long as the LLM emits **Tool Calls** OR there are **Steering** messages queued.
    *   A "Turn" consists of sending the current context to the LLM and processing its response.
3.  **Inner Logic (Step Execution)**:
    *   **Context Preparation**: `AgentMessage` history is converted to LLM-compatible `Message` format.
    *   **Assistant Response**: The LLM streams its response. If the response contains tool calls, they are collected.
    *   **Tool Execution**: Each tool call is executed sequentially.
    *   **Steering Interruption**: Between *each* tool execution, the loop checks for "Steer" messages from the user. If a steering message exists, remaining tool calls in the current turn are skipped to prioritize the user's new input.

### 3.2. Interaction & Waiting for Input
The agent "waits" for user input only when:
*   The current turn has finished (all tool calls executed or skipped).
*   The steering queue is empty.
*   The follow-up queue is empty.
*   The LLM has stopped without requesting more tools.

At this point, the `Run` method completes, and the orchestration layer waits for the next `Prompt()` call from the user interface.

### 3.3. Tool Execution Flow
1.  **Selection**: Filter LLM response for `tool_use` content blocks.
2.  **Dispatch**: For each tool call:
    *   Lookup the tool implementation in the `ToolRegistry`.
    *   Validate arguments against the tool's schema.
    *   Emit `tool_execution_start` event.
    *   Execute the tool's `Run` logic (e.g., file read, bash execution).
    *   Record result (success or error) and emit `tool_execution_end`.
3.  **Result Injection**: Concatenate all tool results into a single `toolResult` message and append it to the conversation history.

### 3.4. Steering vs. Follow-up
*   **Steer (Immediate)**: High-priority interrupt. It injects a user message *before* the next LLM turn and skips any pending tool results from the current assistant message. Use case: "Stop what you're doing and look at this error instead."
*   **Follow-up (Queued)**: Low-priority continuation. It injects a user message only *after* the agent has naturally completed its current line of thought (no more tools requested). Use case: "Once you finish this refactor, run the tests."

### 3.5. Loop Pseudocode

```go
func RunLoop(msg AgentMessage) {
    // 1. Queue initial message
    pendingMessages = [msg]

    // Outer Loop: Autonomous Mult-Turn (Follow-ups)
    for {
        hasMoreToolCalls := true

        // Middle Loop: Current Turn (Tools & Steering)
        for hasMoreToolCalls || len(pendingMessages) > 0 {

            // Inject any pending messages (User prompts, Follow-ups, Steering)
            context.Messages.Append(pendingMessages...)
            pendingMessages = []

            // A. Stream Assistant Response
            stream := llm.Stream(context)
            assistantMsg := stream.FullMessage() // Blocks until done or interrupted
            context.Messages.Append(assistantMsg)

            // B. Handle Tool Calls
            toolCalls := assistantMsg.ToolCalls
            hasMoreToolCalls = len(toolCalls) > 0

            for i, toolCall := range toolCalls {
                // Execute Tool
                result := tools.Execute(toolCall)
                context.Messages.Append(result)

                // C. Check for Steering Interrupts (The "Steering Gap")
                // If user typed something while tool was running, we interrupt immediately.
                if len(steeringQueue) > 0 {
                    pendingMessages = steeringQueue.Drain()
                    
                    // SKIP remaining tool calls in this turn
                    // The loop continues, and these pending messages will be 
                    // injected at the top of the loop, triggering a NEW LLM response.
                    hasMoreToolCalls = true
                    break 
                }
            }
        }

        // D. Check for Follow-up Messages (Autonomous Continuation)
        // If the agent queued a follow-up (e.g., "Run tests next"), process it now.
        if len(followUpQueue) > 0 {
            pendingMessages = followUpQueue.Drain()
            continue // Loop back to start a new turn
        }

        // Loop Ends: No tools, no steering, no follow-ups. Agent is Idle.
        break
    }
}
```

## 4. Go Data Models

### 4.1. Config & Options

```go
package session

import (
	"context"
	"time"
)

// AgentSessionConfig defines the dependencies and settings for an AgentSession.
type AgentSessionConfig struct {
	Agent          Agent          // Low-level agent interface
	Manager        Manager        // Session Manager for persistence
	ModelRegistry  ModelRegistry  // For model discovery and API keys
	ResourceLoader ResourceLoader // For skills and prompt templates
	Cwd            string
}

// PromptOptions controls how a prompt is processed.
type PromptOptions struct {
	ExpandTemplates   bool
	Images            []ImageContent
	StreamingBehavior StreamingBehavior // "steer" or "followUp"
	Source            string            // "interactive", "extension", etc.
}

type StreamingBehavior string

const (
	BehaviorSteer    StreamingBehavior = "steer"
	BehaviorFollowUp StreamingBehavior = "followUp"
)
```

### 4.2. Events

```go
type EventType string

const (
	EventAutoCompactionStart EventType = "auto_compaction_start"
	EventAutoCompactionEnd   EventType = "auto_compaction_end"
	EventAutoRetryStart      EventType = "auto_retry_start"
	EventAutoRetryEnd        EventType = "auto_retry_end"
)

type SessionEvent struct {
	Type      EventType
	Timestamp time.Time
	Payload   any // Detailed event data
}
```

## 5. AgentSession API

```go
type AgentSession interface {
	// --- Prompting ---

	// Prompt sends a message to the agent.
	// It handles skill/template expansion and manages streaming queues.
	Prompt(ctx context.Context, text string, opts PromptOptions) error

	// Steer queues an interrupt message.
	Steer(text string) error

	// FollowUp queues a message for after the current turn.
	FollowUp(text string) error

	// Abort cancels the current agent operation.
	Abort() error

	// --- Model & Thinking ---

	// SetModel switches the active model and validates API keys.
	SetModel(provider, modelID string) error

	// CycleModel moves to the next available model in the registry.
	CycleModel(direction string) (ModelInfo, error)

	// SetThinkingLevel updates the depth of internal reasoning.
	SetThinkingLevel(level string) error

	// --- Compaction & History ---

	// Compact manually triggers a summarization of the current branch.
	Compact(customInstructions string) (CompactionResult, error)

	// NewSession starts a fresh conversation.
	NewSession(parentID string) error

	// SwitchSession loads an existing session file.
	SwitchSession(sessionID string) error

	// Fork branches the current conversation from a specific entry.
	Fork(entryID string) error

	// --- State & Metadata ---

	// State returns the current in-memory agent state.
	State() AgentState

	// Close cleans up resources.
	Close() error
}
```

## 6. Internal Logic

### 6.1. Auto-Compaction
`AgentSession` monitors `TurnEnd` events.
*   **Threshold Trigger**: If `usage.TotalTokens > threshold`, trigger a background compaction.
*   **Overflow Trigger**: If the provider returns a `ContextOverflow` error:
    1.  Remove the error message from the current context.
    2.  Trigger immediate compaction.
    3.  Auto-retry the failed prompt.

### 6.2. Auto-Retry
Transient errors (429, 500, 503, connectivity) trigger exponential backoff:
1.  **Delay**: `baseDelay * 2^attempt`.
2.  **Cleanup**: Remove the error message from the context before retrying.
3.  **Maximums**: Stop after `MaxRetries` and report final failure.

### 6.3. Skill & Template Expansion
Before sending to the agent:
*   **Skills**: `/skill:name` is replaced by the content of the corresponding `.md` file in the skills directory, wrapped in `<skill>` tags.
*   **Templates**: Slash commands matching prompt templates are expanded, inserting arguments into placeholders.

## 7. Bash Orchestration
`AgentSession` provides an `ExecuteBash` wrapper:
1.  **Prefix Injection**: Prepends configured shell prefixes (e.g., for aliases).
2.  **Streaming Output**: Relays chunks to the UI in real-time.
3.  **Context Management**: Handles the special `!!` prefix to exclude command output from the LLM context while still logging it to the session.
4.  **Persistence**: Records the `bashExecution` event in the `SessionManager`.

## 8. Extension Integration
`AgentSession` serves as the primary host for extensions:
*   **Command Registration**: Extensions can register custom slash commands.
*   **Turn Hooks**: `BeforeAgentStart`, `TurnStart`, `TurnEnd` allow extensions to modify prompts or system instructions.
*   **Resource Discovery**: Extensions can provide additional skills, prompts, or tools.

## 9. Persistence Strategy

`AgentSession` is responsible for ensuring that all significant conversation events are persisted to the `SessionManager` as they occur.

*   **Real-time Append**: Instead of saving the entire session at the end of a run, `AgentSession` appends entries to the JSONL log immediately when:
    *   A `message_end` event is received (for User, Assistant, and Tool Result messages).
    *   A bash command completes (`bashExecution`).
    *   A compaction summary is generated (`compaction`).
    *   A user manually bookmarks a node (`label`).
*   **Crash Resilience**: Because the underlying storage is append-only JSONL, the session remains consistent even if the process is terminated mid-turn.
*   **In-Memory Sync**: The `AgentSession` keeps its in-memory message history in sync with the persisted log. If a session is resumed, `AgentSession` calls `SessionManager.GetContext()` to rebuild the exact state (including thinking levels and models).

## 10. Usage Example

```go
package main

import (
	"context"
	"fmt"
	"log"
	
	"session" // hypothetical package
	"agent"   // hypothetical package
)

func main() {
	// 1. Initialize Dependencies
	mgr := session.NewManager("./sessions")
	
	// 2. Load or Create Session (Data Layer)
	sess, err := mgr.ContinueRecent()
	if err != nil {
		sess, _ = mgr.New("")
	}
	defer sess.Close()
	
	// 3. Initialize Agent Orchestrator
	// This layer binds the agent logic to the session persistence
	cfg := session.AgentSessionConfig{
		Manager: sess,
		Agent: agent.New(),
		Cwd: "/path/to/project",
	}
	
	agentSession := session.NewAgentSession(cfg)
	defer agentSession.Close()

	// 4. Interactive Prompt Loop
	ctx := context.Background()
	
	// Send a standard user prompt
	// The AgentSession handles:
	// - Expanding skills/templates
	// - Streaming the response to the LLM (internal loop)
	// - Persisting message_start/message_end events to the session file
	// - Executing tools (e.g., read_file, bash)
	err = agentSession.Prompt(ctx, "Refactor main.go to use the new logger.", session.PromptOptions{})
	if err != nil {
		log.Fatalf("Prompt failed: %v", err)
	}
	
	// 5. Simulate an Interrupt (Steering)
	// If the user presses Ctrl+C or types while the tool is running,
	// we call Steer() to inject a high-priority message.
	// This will cause the internal RunLoop to skip remaining tools and 
	// immediately address this new instruction.
	go func() {
		// Simulate user typing "Wait, check for errors first"
		agentSession.Steer("Wait, check for errors first")
	}()
	
	// 6. Manual Compaction
	// If the context gets too long, we can manually trigger compaction
	result, err := agentSession.Compact("Summarize the debugging steps so far.")
	if err == nil {
		fmt.Printf("Compacted! Kept from ID: %s, Summary: %s\n", result.FirstKeptID, result.Summary)
	}
}
```
