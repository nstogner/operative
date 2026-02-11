package jsonl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mariozechner/coding-agent/session/pkg/store"
)

// Manager implements the store.Manager interface using JSONL files.
type Manager struct {
	rootDir   string
	agentDir  string
	sessDir   string
	eventChan chan string
	mu        sync.RWMutex
	subs      []chan string
}

func (m *Manager) SetSessionStatus(id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	indexPath := filepath.Join(m.sessDir, "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return err
	}

	found := false
	for i := range idx.Sessions {
		if idx.Sessions[i].ID == id {
			idx.Sessions[i].Status = status
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("session %s not found", id)
	}

	updatedData, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, updatedData, 0644)
}

func NewManager(rootDir string) *Manager {
	m := &Manager{
		rootDir:   rootDir,
		agentDir:  filepath.Join(rootDir, "agents"),
		sessDir:   filepath.Join(rootDir, "sessions"),
		eventChan: make(chan string, 100),
	}
	// Improve: error handling here is tricky as constructor doesn't return error.
	// Best effort creation.
	os.MkdirAll(m.agentDir, 0755)
	os.MkdirAll(m.sessDir, 0755)

	go m.broadcastLoop()
	return m
}

// Index represents the index.json structure
type Index struct {
	Sessions []SessionMeta `json:"sessions"`
}

type SessionMeta struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Status    string    `json:"status"`
	AgentID   string    `json:"agent_id"`
	AgentName string    `json:"agent_name"`
	Created   time.Time `json:"created"`
	Modified  time.Time `json:"modified"`
}

func (m *Manager) updateIndex(meta SessionMeta) error {
	indexPath := filepath.Join(m.sessDir, "index.json")

	// Read existing
	var idx Index
	data, err := os.ReadFile(indexPath)
	if err == nil {
		json.Unmarshal(data, &idx)
	}

	// Update or Append
	found := false
	for i, s := range idx.Sessions {
		if s.ID == meta.ID {
			idx.Sessions[i] = meta
			found = true
			break
		}
	}
	if !found {
		idx.Sessions = append(idx.Sessions, meta)
	}

	// Write back
	updatedData, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, updatedData, 0644)
}

func (m *Manager) readIndex() ([]SessionMeta, error) {
	indexPath := filepath.Join(m.sessDir, "index.json")
	data, err := os.ReadFile(indexPath)
	if os.IsNotExist(err) {
		return []SessionMeta{}, nil
	}
	if err != nil {
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return idx.Sessions, nil
}

func (m *Manager) broadcastLoop() {
	for id := range m.eventChan {
		m.mu.RLock()
		for _, sub := range m.subs {
			// Non-blocking send
			select {
			case sub <- id:
			default:
			}
		}
		m.mu.RUnlock()
	}
}

func (m *Manager) Subscribe() <-chan string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan string, 10)
	m.subs = append(m.subs, ch)
	return ch
}

func (m *Manager) publish(id string) {
	select {
	case m.eventChan <- id:
	default:
	}
}

func (m *Manager) NewSession(agentID, parentSessionID string) (store.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Validate Agent
	// Always require an agent. If ID is empty, try "default" or fallback.
	if agentID == "" {
		agentID = "default"
	}

	var agent *store.Agent
	agent, err := m.GetAgent(agentID)
	if err != nil {
		// If requesting "default" and it doesn't exist, try to find ANY agent or create one.
		if agentID == "default" {
			agents, listErr := m.listAgentsLocked()
			if listErr == nil && len(agents) > 0 {
				// Use the first available agent
				agent = &agents[0]
				// We don't change agentID here because the session stores the full agent struct in header.
				// But we should probably track which ID we used?
				// The header stores the Agent struct, so it's fine.
			} else {
				// No agents exist, create a default one
				newDefault := &store.Agent{
					ID:           "default",
					Name:         "Default Agent",
					Instructions: "You are a helpful assistant.",
					Model:        "models/gemini-2.0-flash", // Use a sensible default
				}
				if createErr := m.newAgentLocked(newDefault); createErr != nil {
					return nil, fmt.Errorf("failed to create default agent: %w", createErr)
				}
				agent = newDefault
			}
		} else {
			return nil, fmt.Errorf("failed to get agent %s: %w", agentID, err)
		}
	}

	if err := os.MkdirAll(m.sessDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	id := uuid.New().String()
	path := filepath.Join(m.sessDir, id+".jsonl")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create session file: %w", err)
	}

	s := &Session{
		id:         id,
		filePath:   path,
		entries:    make(map[string]store.Entry),
		fileHandle: f,
		labels:     make(map[string]string),
		notify:     m.publish,
	}

	now := time.Now()
	header := store.Header{
		Type:          store.TypeSession,
		ID:            id,
		Agent:         *agent,
		Version:       1,
		ParentSession: parentSessionID,
		CreatedAt:     now,
	}
	s.header = header

	if err := s.writeLine(header); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to write session header: %w", err)
	}

	// 2. Append System Prompt logic REMOVED. System prompt is now dynamic based on Header.Agent.

	// Update Index
	meta := SessionMeta{
		ID:        id,
		Path:      path,
		Status:    store.SessionStatusActive,
		AgentID:   agent.ID,
		AgentName: agent.Name,
		Created:   now,
		Modified:  now,
	}
	if err := m.updateIndex(meta); err != nil {
		slog.Error("Failed to update session index", "error", err)
	}

	return s, nil
}

func (m *Manager) LoadSession(id string) (store.Session, error) {
	path := filepath.Join(m.sessDir, id+".jsonl")
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open session file: %w", err)
	}

	s := &Session{
		filePath:   path,
		entries:    make(map[string]store.Entry),
		fileHandle: f,
		labels:     make(map[string]string),
		notify:     m.publish,
	}

	if err := m.loadEntries(s); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to load entries: %w", err)
	}

	return s, nil
}

func (m *Manager) ContinueRecent() (store.Session, error) {
	infos, err := m.ListSessions()
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		return nil, fmt.Errorf("no sessions found in %s", m.sessDir)
	}
	return m.LoadSession(infos[0].ID)
}

func (m *Manager) ForkFrom(id string) (store.Session, error) {
	source, err := m.LoadSession(id)
	if err != nil {
		return nil, err
	}
	defer source.Close()

	// TODO: Get Agent ID from source session header?
	// For now, load header separately or assume we need to pass agent ID.
	// Let's assume we reuse the agent from the source session if possible.
	// Since we don't have GetHeader easily exposed on interface, we might need to look at logic.
	// But ForkFrom implies same agent? Or new session logic?
	// For simplicity, let's assume "default" agent or reuse.
	// Actually, we need to read the source header.

	// Quick fix: peek at file or add GetHeader to interface.
	// Or cast to implementation.
	jsonlSource := source.(*Session)
	var agentID string

	// We already loaded entries. The header info is not stored in entries map explicitly as struct,
	// but we can parse it from file start.
	// Ideally Session interface should have AgentID().
	// For now, let's just peek the file again or trust simple logic.
	// Implementation hack: Read first line of source file
	if _, err := jsonlSource.fileHandle.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(jsonlSource.fileHandle)
	if scanner.Scan() {
		var h store.Header
		json.Unmarshal(scanner.Bytes(), &h)
		agentID = h.Agent.ID
	}

	// If empty agentID, we might fail NewSession.
	// Let's allow empty if NewSession handles it? NewSession tries GetAgent.
	// If old session has no agent, we might need a fallback.
	if agentID == "" {
		// Fallback: create a default agent or error?
		// Let's rely on caller to migrate or handle.
		// For now, if empty, we might pass empty and let GetAgent fail or NewSession fail.
	}

	dest, err := m.NewSession(agentID, source.ID())
	if err != nil {
		return nil, err
	}

	if _, err := jsonlSource.fileHandle.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	scanner = bufio.NewScanner(jsonlSource.fileHandle)
	scanner.Scan() // skip header again

	for scanner.Scan() {
		var e store.Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		// Skip system prompt if we just added a new one?
		// Or keep history exactly?
		// If we reuse Agent, we probably just added a fresh system prompt.
		// If source has a system prompt, we might duplicate it.
		// For now, simple copy.
		if err := dest.Append(e); err != nil {
			dest.Close()
			return nil, err
		}
	}

	return dest, nil
}

func (m *Manager) ListSessions() ([]store.SessionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metas, err := m.readIndex()
	if err != nil {
		return nil, err
	}

	var infos []store.SessionInfo
	for _, meta := range metas {
		infos = append(infos, store.SessionInfo{
			ID:        meta.ID,
			Path:      meta.Path,
			Status:    meta.Status,
			AgentID:   meta.AgentID,
			AgentName: meta.AgentName,
			Created:   meta.Created,
			Modified:  meta.Modified,
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Modified.After(infos[j].Modified)
	})

	return infos, nil
}

// Agent Methods

// Internal helper without locking
func (m *Manager) listAgentsLocked() ([]store.Agent, error) {
	// Ensure agent dir exists
	if _, err := os.Stat(m.agentDir); os.IsNotExist(err) {
		return []store.Agent{}, nil
	}

	entries, err := os.ReadDir(m.agentDir)
	if err != nil {
		return nil, err
	}

	var agents []store.Agent
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			data, err := os.ReadFile(filepath.Join(m.agentDir, e.Name()))
			if err != nil {
				continue // Skip bad files
			}
			var a store.Agent
			if err := json.Unmarshal(data, &a); err == nil {
				agents = append(agents, a)
			}
		}
	}
	return agents, nil
}

// Internal helper without locking
func (m *Manager) newAgentLocked(a *store.Agent) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}

	path := filepath.Join(m.agentDir, a.ID+".json")
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func (m *Manager) NewAgent(a *store.Agent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.newAgentLocked(a)
}

func (m *Manager) UpdateAgent(a *store.Agent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if a.ID == "" {
		return fmt.Errorf("agent ID is required for update")
	}

	path := filepath.Join(m.agentDir, a.ID+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("agent %s not found", a.ID)
	}

	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func (m *Manager) DeleteAgent(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(m.agentDir, id+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("agent %s not found", id)
	}

	return os.Remove(path)
}

func (m *Manager) ListAgents() ([]store.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listAgentsLocked()
}

func (m *Manager) GetAgent(id string) (*store.Agent, error) {
	// If ID is empty, return a default empty agent or specific error?
	// Let's allow checking for a "default" agent file if ID is empty/default.

	path := filepath.Join(m.agentDir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var a store.Agent
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (m *Manager) loadEntries(s *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.fileHandle.Seek(0, io.SeekStart); err != nil {
		return err
	}

	scanner := bufio.NewScanner(s.fileHandle)
	var lastID string

	if scanner.Scan() {
		var h store.Header
		if err := json.Unmarshal(scanner.Bytes(), &h); err != nil {
			return fmt.Errorf("failed to unmarshal header: %w", err)
		}
		s.id = h.ID
		s.header = h
	}

	for scanner.Scan() {
		var e store.Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		s.entries[e.ID] = e
		lastID = e.ID

		if e.Type == store.TypeLabel && e.Label != nil {
			s.labels[e.Label.TargetID] = e.Label.Label
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	s.leafID = lastID
	return nil
}
