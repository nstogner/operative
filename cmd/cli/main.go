// Package main implements a CLI for the coding agent.
//
// Usage:
//
//	export GEMINI_API_KEY="your-api-key"
//	go run cmd/cli/main.go
//
// Commands:
//
//	/exit - Exit the program
//	<message> - Send a message to the agent
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/models/gemini"
	"github.com/mariozechner/coding-agent/session/pkg/runner"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox/docker"
	"github.com/mariozechner/coding-agent/session/pkg/session"
	"github.com/mariozechner/coding-agent/session/pkg/session/jsonl"
)

var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#25A065")).
			Padding(0, 1)

	senderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("5")).
			Bold(true)

	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("2")).
			Bold(true)

	messageStyle = lipgloss.NewStyle().PaddingLeft(2)

	cursorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	selectedItemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Padding(0, 1) // Red
)

type state int

const (
	stateMenu state = iota
	stateSelectingSession
	stateSelectingModel
	stateChatting
	stateConfirmExit
)

type errMsg struct{ err error }
type sessionUpdateMsg string
type runnerErrorMsg struct{ err error }

type model struct {
	// ... (fields same as before)
	ctx            context.Context
	modelProvider  models.ModelProvider
	sessManager    session.Manager
	currentSess    session.Session
	runner         *runner.Runner
	sandboxManager sandbox.Manager
	updates        <-chan string

	// State
	state             state
	availableModels   []string
	availableSessions []session.SessionInfo
	cursor            int
	listOffset        int
	width             int
	height            int
	err               error

	// UI Components
	viewport viewport.Model
	textarea textarea.Model

	// Data
	messages []session.Entry
	renderer *glamour.TermRenderer
}

func initialModel(ctx context.Context, provider models.ModelProvider, manager session.Manager, modelsList []string) model {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()
	ta.Prompt = "┃ "
	ta.CharLimit = 280

	ta.SetWidth(80)
	ta.SetHeight(3)

	// Remove cursor line styling
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false

	vp := viewport.New(80, 20)
	vp.SetContent("Welcome! Select an option.")

	// Use "light" style to avoid terminal queries that leak into input
	r, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("light"),
		glamour.WithWordWrap(80),
	)

	// Check for existing sessions
	startState := stateMenu
	sessions, err := manager.List()
	if err == nil && len(sessions) == 0 {
		startState = stateSelectingModel
	}

	return model{
		ctx:             ctx,
		modelProvider:   provider,
		sessManager:     manager,
		availableModels: modelsList,
		state:           startState,
		viewport:        vp,
		textarea:        ta,
		renderer:        r,
	}
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	var tiCmd, vpCmd tea.Cmd
	// This prevents the Enter key used for menu selection from leaking into the textarea.
	switch msg.(type) {
	case tea.KeyMsg:
		if m.state == stateChatting {
			m.textarea, tiCmd = m.textarea.Update(msg)
			cmds = append(cmds, tiCmd)
		}
	default:
		m.textarea, tiCmd = m.textarea.Update(msg)
		cmds = append(cmds, tiCmd)
	}

	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.textarea.SetWidth(msg.Width)
		m.viewport.Height = msg.Height - m.textarea.Height() - 2 // Header + Margin
		m.viewport.Height = msg.Height - m.textarea.Height() - 2 // Header + Margin
		if m.viewport.Height < 0 {
			m.viewport.Height = 0
		}
		m.viewport.YPosition = 2

		// Recreate renderer with new width
		// Using standard style avoids "Querying terminal..." escape sequences leaking into input
		m.renderer, _ = glamour.NewTermRenderer(
			glamour.WithStandardStyle("light"),
			glamour.WithWordWrap(m.width-4),
		)

		// Re-clamp listOffset to ensure cursor remains visible after resize
		maxViewable := m.height - 7
		if maxViewable < 1 {
			maxViewable = 1
		}
		if m.cursor < m.listOffset {
			m.listOffset = m.cursor
		}
		if m.cursor >= m.listOffset+maxViewable {
			m.listOffset = m.cursor - maxViewable + 1
		}
		if m.listOffset < 0 {
			m.listOffset = 0
		}

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.currentSess != nil {
				m.state = stateConfirmExit
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyEsc:
			if m.state == stateConfirmExit {
				m.state = stateChatting
				return m, nil
			}
			if m.currentSess != nil {
				m.state = stateConfirmExit
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyEnter:
			if m.state == stateMenu {
				if m.cursor == 0 {
					// New Session
					m.state = stateSelectingModel
					m.cursor = 0
					m.listOffset = 0
				} else {
					// Continue Session
					sessions, err := m.sessManager.List()
					if err != nil {
						m.err = err
					} else if len(sessions) == 0 {
						m.err = fmt.Errorf("no existing sessions found")
					} else {
						m.availableSessions = sessions
						m.state = stateSelectingSession
						m.cursor = 0
						m.listOffset = 0
					}
				}
			} else if m.state == stateSelectingModel {
				m, cmd := m.selectModel()
				return m, cmd
			} else if m.state == stateSelectingSession {
				m, cmd := m.selectSession()
				return m, cmd
			} else if m.state == stateChatting {
				m.err = nil // Clear error on new message
				m, cmd := m.sendMessage()
				return m, cmd
			}
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.listOffset {
					m.listOffset = m.cursor
				}
			}
		case tea.KeyDown:
			var maxCursor int
			switch m.state {
			case stateMenu:
				maxCursor = 1 // 2 options
			case stateSelectingModel:
				maxCursor = len(m.availableModels) - 1
			case stateSelectingSession:
				maxCursor = len(m.availableSessions) - 1
			}
			if m.cursor < maxCursor {
				m.cursor++
				// Calculate max viewable items (Total height - header - footer)
				// Header: ~3 lines, Footer: ~3 lines
				maxViewable := m.height - 7
				if maxViewable < 1 {
					maxViewable = 1
				}
				if m.cursor >= m.listOffset+maxViewable {
					m.listOffset = m.cursor - maxViewable + 1
				}
			}
		default:
			if m.state == stateConfirmExit {
				switch msg.String() {
				case "y", "Y":
					// End Session
					return m, tea.Sequence(
						m.endSessionCmd(),
						tea.Quit,
					)
				case "n", "N":
					// Leave Running
					return m, tea.Quit
				}
			}
		}

	case sessionUpdateMsg:
		slog.Debug("TUI received update for session", "sessionID", msg)
		// Reload session messages
		if m.currentSess != nil && string(msg) == m.currentSess.ID() {
			slog.Debug("Reloading messages...")
			cmds = append(cmds, m.reloadMessages(), waitForUpdate(m.updates))
		} else {
			cmds = append(cmds, waitForUpdate(m.updates))
		}

	case updateViewMsg:
		slog.Debug("Updating view", "contentLen", len(msg.content))
		if m.currentSess != nil {
			m.currentSess.Close()
		}
		m.currentSess = msg.sess
		m.viewport.SetContent(msg.content)
		m.viewport.GotoBottom()

	case errMsg:
		m.err = msg.err

	case runnerErrorMsg:
		slog.Debug("TUI received runner error", "error", msg.err)
		m.err = msg.err
		cmds = append(cmds, waitForRunnerError(m.runner.ErrorChan))
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.state == stateMenu {
		header := titleStyle.Render("Main Menu")

		options := []string{"New Session", "Continue Session"}
		var optionsView []string
		for i, choice := range options {
			cursor := " "
			if m.cursor == i {
				cursor = ">"
				choice = selectedItemStyle.Render(choice)
			}
			optionsView = append(optionsView, fmt.Sprintf("%s %s", cursorStyle.Render(cursor), choice))
		}

		list := lipgloss.JoinVertical(lipgloss.Left, optionsView...)
		footer := "Press Enter to select, Esc to quit."

		return lipgloss.JoinVertical(lipgloss.Left, header, "", list, "", footer)
	}

	if m.state == stateSelectingSession {
		header := titleStyle.Render("Select Session")

		maxViewable := m.height - 7
		if maxViewable < 1 {
			maxViewable = 1
		}

		start := m.listOffset
		end := start + maxViewable
		if end > len(m.availableSessions) {
			end = len(m.availableSessions)
		}

		var optionsView []string
		for i := start; i < end; i++ {
			choice := m.availableSessions[i]
			cursor := " "
			line := fmt.Sprintf("%s (%s)", choice.ID, choice.Modified.Format(time.RFC822))
			if m.cursor == i {
				cursor = ">"
				line = selectedItemStyle.Render(line)
			}
			optionsView = append(optionsView, fmt.Sprintf("%s %s", cursorStyle.Render(cursor), line))
		}

		list := lipgloss.JoinVertical(lipgloss.Left, optionsView...)
		footer := "Press Enter to select, Esc to quit."

		return lipgloss.JoinVertical(lipgloss.Left, header, "", list, "", footer)
	}

	if m.state == stateSelectingModel {
		header := titleStyle.Render("Select Model")

		maxViewable := m.height - 7
		if maxViewable < 1 {
			maxViewable = 1
		}

		start := m.listOffset
		end := start + maxViewable
		if end > len(m.availableModels) {
			end = len(m.availableModels)
		}

		var optionsView []string
		for i := start; i < end; i++ {
			choice := m.availableModels[i]
			cursor := " "
			if m.cursor == i {
				cursor = ">"
				choice = selectedItemStyle.Render(choice)
			}
			optionsView = append(optionsView, fmt.Sprintf("%s %s", cursorStyle.Render(cursor), choice))
		}

		list := lipgloss.JoinVertical(lipgloss.Left, optionsView...)
		footer := "Press Enter to select, Esc to quit."

		return lipgloss.JoinVertical(lipgloss.Left, header, "", list, "", footer)
	}

	if m.state == stateConfirmExit {
		header := titleStyle.Render("Confirm Exit")
		prompt := "End Session? (y/n)"
		subtext := "Ending the session will remove the sandbox."

		return lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			"",
			prompt,
			subtext,
		)
	}

	var errorView string
	if m.err != nil {
		errorView = errorStyle.Width(m.width).Render(fmt.Sprintf("Error: %v", m.err))
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("Gemini Agent"),
		"",
		m.viewport.View(),
		"",
		errorView,
		m.textarea.View(),
	)
}

// Actions

func (m model) selectModel() (model, tea.Cmd) {
	selected := m.availableModels[m.cursor]

	// Create Sandbox Manager (Simplified)
	sbMgr, err := docker.New()
	if err != nil {
		slog.Error("Failed to initialize sandbox manager", "error", err)
		return m, func() tea.Msg { return errMsg{err} }
	}

	// Initialize Runner
	m.runner = runner.New(m.sessManager, m.modelProvider, selected, sbMgr)
	m.sandboxManager = sbMgr

	// Start Runner in background
	go func() {
		if err := m.runner.Start(m.ctx); err != nil && err != context.Canceled {
			slog.Error("Runner stopped", "error", err)
		}
	}()

	// Create Session
	sess, err := m.sessManager.New("")
	if err != nil {
		return m, func() tea.Msg { return errMsg{err} }
	}
	m.currentSess = sess
	return m.enterChat()
}

func (m model) selectSession() (model, tea.Cmd) {
	selectedSession := m.availableSessions[m.cursor]

	// Use default model or ask? For now default to first available or hardcode.
	// ideally we persist model choice in session metadata. Use first one for now.
	modelName := m.availableModels[0]

	// Create Sandbox Manager
	sbMgr, err := docker.New()
	if err != nil {
		return m, func() tea.Msg { return errMsg{err} }
	}

	m.runner = runner.New(m.sessManager, m.modelProvider, modelName, sbMgr)
	m.sandboxManager = sbMgr

	go func() {
		if err := m.runner.Start(m.ctx); err != nil && err != context.Canceled {
			slog.Error("Runner stopped", "error", err)
		}
	}()

	sess, err := m.sessManager.Load(selectedSession.ID)
	if err != nil {
		return m, func() tea.Msg { return errMsg{err} }
	}
	m.currentSess = sess

	return m.enterChat()
}

func (m model) enterChat() (model, tea.Cmd) {
	// Subscribe to updates
	m.updates = m.sessManager.Subscribe()

	m.state = stateChatting
	m.textarea.Placeholder = "Type a message..."
	m.textarea.Focus()

	// Initial empty load + start listening
	return m, tea.Batch(
		m.reloadMessages(),
		waitForUpdate(m.updates),
		waitForRunnerError(m.runner.ErrorChan),
	)
}

func (m model) sendMessage() (model, tea.Cmd) {
	v := m.textarea.Value()
	if v == "" {
		return m, nil
	}

	if v == "/exit" {
		m.state = stateConfirmExit
		return m, nil
	}

	// Clear input
	m.textarea.Reset()

	// Append Message
	return m, func() tea.Msg {
		_, err := m.currentSess.AppendMessage(session.RoleUser, []session.Content{
			{Type: session.ContentTypeText, Text: &session.TextContent{Content: v}},
		})
		if err != nil {
			return errMsg{err}
		}
		// The append will trigger an event via Manager, which we listen to
		return nil
	}
}

func (m model) endSessionCmd() tea.Cmd {
	return func() tea.Msg {
		if m.currentSess != nil {
			if err := m.sessManager.SetStatus(m.currentSess.ID(), session.SessionStatusEnded); err != nil {
				slog.Error("Failed to set session status", "error", err)
			}
			if m.sandboxManager != nil {
				if err := m.sandboxManager.Stop(m.ctx, m.currentSess.ID()); err != nil {
					slog.Error("Failed to stop sandbox", "error", err)
				}
			}
		}
		return nil
	}
}

type updateViewMsg struct {
	content string
	sess    session.Session
}

func (m model) reloadMessages() tea.Cmd {
	return func() tea.Msg {
		// Create a temporary read-only view to get the latest state from disk
		sess, err := m.sessManager.Load(m.currentSess.ID())
		if err != nil {
			return errMsg{err}
		}

		entries, err := sess.GetContext()
		if err != nil {
			sess.Close()
			return errMsg{err}
		}

		slog.Info("Loaded entries from session", "count", len(entries))

		slog.Info("Loaded entries from session", "count", len(entries))

		var sb strings.Builder
		for _, e := range entries {
			if e.Message != nil {
				if len(e.Message.Content) == 0 {
					continue
				}
				role := string(e.Message.Role)
				var content string
				if len(e.Message.Content) > 0 {
					if e.Message.Content[0].Text != nil {
						rawContent := e.Message.Content[0].Text.Content
						// Check if it's potentially markdown or just plain text
						// Use the persistent renderer
						var rendered string
						var err error
						if m.renderer != nil {
							rendered, err = m.renderer.Render(rawContent)
						} else {
							err = fmt.Errorf("renderer not ready")
						}

						if err != nil {
							content = rawContent // Fallback
						} else {
							content = rendered
						}
					} else if e.Message.Content[0].ToolUse != nil {
						// Render tool usage
						toolUse := e.Message.Content[0].ToolUse
						content = fmt.Sprintf("[Tool Usage: %s]", toolUse.Name)
						if code, ok := toolUse.Input["code"].(string); ok {
							content += fmt.Sprintf("\n\n%s", code)
						}
					} else if e.Message.Content[0].ToolResult != nil {
						// Render tool result
						result := e.Message.Content[0].ToolResult
						status := "Success"
						if result.IsError {
							status = "Error"
						}
						content = fmt.Sprintf("[%s: %s]\n%s", status, result.ToolUseID, result.Content)
					}
				}

				// Titles
				if e.Message.Role == session.RoleUser {
					sb.WriteString(userStyle.Render("User: "))
				} else if e.Message.Role == session.RoleAssistant {
					sb.WriteString(senderStyle.Render("AI: "))
				} else {
					sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(role + ": "))
				}
				sb.WriteString("\n")

				// Content - glamour adds its own margins usually, but let's just append
				sb.WriteString(content)
				sb.WriteString("\n")
			}
		}

		return updateViewMsg{content: sb.String(), sess: sess}
	}
}

func waitForRunnerError(ch <-chan error) tea.Cmd {
	return func() tea.Msg {
		err, ok := <-ch
		if !ok {
			return nil
		}
		return runnerErrorMsg{err}
	}
}

func waitForUpdate(sub <-chan string) tea.Cmd {
	return func() tea.Msg {
		id, ok := <-sub
		if !ok {
			return nil
		}
		return sessionUpdateMsg(id)
	}
}

// --- Main ---

func main() {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		fmt.Println("Error: GEMINI_API_KEY environment variable not set.")
		os.Exit(1)
	}

	ctx := context.Background()
	// Cancel context on exit is handled by OS, but let's be clean
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 1. Setup Logging
	f, err := os.OpenFile("agent.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}
	defer f.Close()

	// Initialize slog
	logLevel := slog.LevelInfo
	if lv := os.Getenv("LOG_LEVEL"); lv != "" {
		switch strings.ToUpper(lv) {
		case "TRACE":
			logLevel = gemini.LevelTrace
		case "DEBUG":
			logLevel = slog.LevelDebug
		case "INFO":
			logLevel = slog.LevelInfo
		case "WARN":
			logLevel = slog.LevelWarn
		case "ERROR":
			logLevel = slog.LevelError
		}
	}
	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: logLevel})
	slog.SetDefault(slog.New(handler))
	slog.Info("Logging initialized", "level", logLevel)

	// 2. Initialize Model
	geminiModel, err := gemini.New(ctx, apiKey)
	if err != nil {
		slog.Error("Failed to initialize Gemini model", "error", err)
		os.Exit(1)
	}

	// 2. Select Model List
	modelsList, err := geminiModel.List(ctx)
	if err != nil {
		slog.Error("Failed to list models", "error", err)
		os.Exit(1)
	}
	if len(modelsList) == 0 {
		slog.Info("No models available.")
		os.Exit(1)
	}

	// 3. Initialize Manager
	mgr := jsonl.NewManager("./sessions")

	// 4. Start Program
	p := tea.NewProgram(initialModel(ctx, geminiModel, mgr, modelsList))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}

// --- Mock Model Implementation (Preserved) ---

// MockModel helper for the CLI to run without a real LLM
type MockModel struct{}

func (m *MockModel) List(ctx context.Context) ([]string, error) {
	return []string{"mock-model"}, nil
}

func (m *MockModel) Stream(ctx context.Context, modelName string, messages []models.AgentMessage) (models.ModelStream, error) {
	// Simple echo mock with tool capability
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages")
	}
	lastMsg := messages[len(messages)-1]
	responseText := fmt.Sprintf("Echo from %s: %s", modelName, lastMsg.Content[0].Text.Content)

	// If user says "tool", trigger a tool call
	var content []session.Content
	if strings.Contains(responseText, "tool") {
		content = []session.Content{
			{
				Type: session.ContentTypeToolUse,
				ToolUse: &session.ToolUseContent{
					ID:    "call-1",
					Name:  "example-tool",
					Input: map[string]any{"arg": "value"},
				},
			},
		}
	} else {
		content = []session.Content{
			{
				Type: session.ContentTypeText,
				Text: &session.TextContent{Content: responseText},
			},
		}
	}

	return &MockStream{
		Msg: models.AgentMessage{
			Role:    session.RoleAssistant,
			Content: content,
		},
	}, nil
}

type MockStream struct {
	Msg models.AgentMessage
}

func (s *MockStream) FullMessage() (models.AgentMessage, error) {
	return s.Msg, nil
}
func (s *MockStream) Close() error { return nil }
