package jsonl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mariozechner/coding-agent/session/pkg/store"
)

// Session implements the store.Session interface using a JSONL file.
type Session struct {
	mu         sync.RWMutex
	id         string
	filePath   string
	entries    map[string]store.Entry // ID -> Entry lookup
	leafID     string                 // Current tip of the tree
	fileHandle *os.File
	labels     map[string]string // EntryID -> Current Label
	notify     func(string)
	header     store.Header
}

func (s *Session) ID() string     { return s.id }
func (s *Session) Path() string   { return s.filePath }
func (s *Session) LeafID() string { return s.leafID }

// Header returns the session metadata.
// Note: In a real implementation, we might want to cache this or re-read it.
// For now, we rely on the fact that NewSession sets it up.
// But wait, s.entries doesn't contain the header.
// We need to store the header in the struct or read it from file.
// Let's modify the struct to store it.
func (s *Session) Header() store.Header {
	// TODO: Store header in struct during load/creation.
	// For now, let's read it from file effectively or store it.
	// We'll update struct in a separate edit if needed, or just read start of file?
	// Reading file is safer but slower.
	// Let's assume we update struct to hold it.
	return s.header
}

// Append persists a generic entry and advances the leaf pointer.
func (s *Session) Append(e store.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e.ParentID == nil && s.leafID != "" {
		pid := s.leafID
		e.ParentID = &pid
	}

	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	if err := s.writeLine(e); err != nil {
		return err
	}

	s.entries[e.ID] = e
	s.leafID = e.ID

	if e.Type == store.TypeLabel && e.Label != nil {
		s.labels[e.Label.TargetID] = e.Label.Label
	}

	if s.notify != nil {
		s.notify(s.id)
	}

	return nil
}

func (s *Session) AppendMessage(role store.MessageRole, content []store.Content) (string, error) {
	id := uuid.New().String()
	e := store.Entry{
		Type: store.TypeMessage,
		ID:   id,
		Message: &store.MessageEntry{
			Role:    role,
			Content: content,
		},
	}
	if err := s.Append(e); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Session) AppendThinkingLevelChange(level string) (string, error) {
	id := uuid.New().String()
	e := store.Entry{
		Type: store.TypeThinkingLevel,
		ID:   id,
		ThinkingLevel: &store.ThinkingLevelEntry{
			ThinkingLevel: level,
		},
	}
	if err := s.Append(e); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Session) AppendModelChange(provider, modelID string) (string, error) {
	id := uuid.New().String()
	e := store.Entry{
		Type: store.TypeModelChange,
		ID:   id,
		ModelChange: &store.ModelChangeEntry{
			Provider: provider,
			ModelID:  modelID,
		},
	}
	if err := s.Append(e); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Session) AppendCompaction(summary, firstKeptID string, tokens int) (string, error) {
	id := uuid.New().String()
	e := store.Entry{
		Type: store.TypeCompaction,
		ID:   id,
		Compaction: &store.CompactionEntry{
			Summary:          summary,
			FirstKeptEntryID: firstKeptID,
			TokensBefore:     tokens,
		},
	}
	if err := s.Append(e); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Session) AppendSessionInfo(name string) (string, error) {
	id := uuid.New().String()
	e := store.Entry{
		Type: store.TypeSessionInfo,
		ID:   id,
		SessionInfo: &store.SessionInfoEntry{
			Name: name,
		},
	}
	if err := s.Append(e); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Session) AppendCustomEntry(customType string, data map[string]any) (string, error) {
	id := uuid.New().String()
	e := store.Entry{
		Type: store.TypeCustom,
		ID:   id,
		Custom: &store.CustomEntry{
			CustomType: customType,
			Data:       data,
		},
	}
	if err := s.Append(e); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Session) SetLabel(targetID string, label string) (string, error) {
	id := uuid.New().String()
	e := store.Entry{
		Type: store.TypeLabel,
		ID:   id,
		Label: &store.LabelEntry{
			TargetID: targetID,
			Label:    label,
		},
	}
	if err := s.Append(e); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Session) Branch(entryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.entries[entryID]; !ok && entryID != "" {
		return fmt.Errorf("entry not found: %s", entryID)
	}

	s.leafID = entryID
	return nil
}

func (s *Session) BranchWithSummary(branchFromID string, summary string) (string, error) {
	if err := s.Branch(branchFromID); err != nil {
		return "", err
	}

	id := uuid.New().String()
	e := store.Entry{
		Type: store.TypeBranchSummary,
		ID:   id,
		BranchSummary: &store.BranchSummaryEntry{
			Summary: summary,
			FromID:  branchFromID,
		},
	}
	if err := s.Append(e); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Session) CreateBranchedSession(leafID string) (string, error) {
	// Root dir is two levels up from session file (sessions/id.jsonl)
	rootDir := filepath.Dir(filepath.Dir(s.filePath))

	// Create a new manager to create the session.
	// NOTE: This assumes standard directory structure.
	m := NewManager(rootDir)

	// TODO: AgentID should be propagated.
	// We read it from the current session header.
	agentID := s.header.Agent.ID

	newS, err := m.NewSession(agentID, s.id)
	if err != nil {
		return "", err
	}
	defer newS.Close()

	s.mu.RLock()
	var path []store.Entry
	currID := leafID
	for currID != "" {
		e, ok := s.entries[currID]
		if !ok {
			s.mu.RUnlock()
			return "", fmt.Errorf("broken path at %s", currID)
		}
		path = append([]store.Entry{e}, path...)
		if e.ParentID == nil {
			break
		}
		currID = *e.ParentID
	}
	s.mu.RUnlock()

	for _, e := range path {
		if err := newS.Append(e); err != nil {
			return "", err
		}
	}

	return newS.ID(), nil
}

func (s *Session) GetContext() ([]store.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var fullPath []store.Entry
	currID := s.leafID

	for currID != "" {
		e, ok := s.entries[currID]
		if !ok {
			return nil, fmt.Errorf("broken parent link: %s", currID)
		}
		fullPath = append([]store.Entry{e}, fullPath...)

		if e.ParentID == nil {
			break
		}
		currID = *e.ParentID
	}

	var mostRecentCompaction *store.CompactionEntry
	compactionIndex := -1

	for i := len(fullPath) - 1; i >= 0; i-- {
		if fullPath[i].Type == store.TypeCompaction {
			mostRecentCompaction = fullPath[i].Compaction
			compactionIndex = i
			break
		}
	}

	if mostRecentCompaction == nil {
		return fullPath, nil
	}

	resolved := []store.Entry{fullPath[compactionIndex]}
	firstKeptID := mostRecentCompaction.FirstKeptEntryID
	include := false
	for _, e := range fullPath {
		if e.ID == firstKeptID {
			include = true
		}
		if include && e.Type != store.TypeCompaction {
			resolved = append(resolved, e)
		}
	}

	return resolved, nil
}

func (s *Session) GetTree() ([]store.TreeNode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	byParent := make(map[string][]store.Entry)
	var roots []store.Entry

	for _, e := range s.entries {
		if e.ParentID == nil {
			roots = append(roots, e)
		} else {
			byParent[*e.ParentID] = append(byParent[*e.ParentID], e)
		}
	}

	var build func(store.Entry) store.TreeNode
	build = func(e store.Entry) store.TreeNode {
		node := store.TreeNode{
			Entry: e,
			Label: s.labels[e.ID],
		}
		children := byParent[e.ID]
		sort.Slice(children, func(i, j int) bool {
			return children[i].Timestamp.Before(children[j].Timestamp)
		})

		for _, child := range children {
			node.Children = append(node.Children, build(child))
		}
		return node
	}

	var tree []store.TreeNode
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].Timestamp.Before(roots[j].Timestamp)
	})
	for _, r := range roots {
		tree = append(tree, build(r))
	}

	return tree, nil
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fileHandle != nil {
		return s.fileHandle.Close()
	}
	return nil
}

func (s *Session) writeLine(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := s.fileHandle.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Session) Refresh() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reset file pointer to start
	if _, err := s.fileHandle.Seek(0, 0); err != nil {
		return err
	}

	scanner := bufio.NewScanner(s.fileHandle)

	// Skip header (first line)
	if scanner.Scan() {
		// Verify header logic? Or just skip.
		// Constructing s.header happens in loadEntries usually.
		// Let's re-parse just in case version changed? Unlikely.
	}

	var lastID string
	// Re-read assignments
	for scanner.Scan() {
		var e store.Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // skip bad lines
		}
		// Update or add
		s.entries[e.ID] = e
		lastID = e.ID

		if e.Type == store.TypeLabel && e.Label != nil {
			s.labels[e.Label.TargetID] = e.Label.Label
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Update leaf to the last read ID.
	// Note: If we added entries in memory that aren't on disk yet, this might overrule them?
	// But in this architecture, we append to file immediately.
	// However, if we are in the middle of a transaction...
	// Ideally Refresh is called when we know there are external updates.
	if lastID != "" {
		s.leafID = lastID
	}

	return nil
}
