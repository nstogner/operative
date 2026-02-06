package jsonl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mariozechner/coding-agent/session/pkg/session"
)

// Session implements the session.Session interface using a JSONL file.
type Session struct {
	mu         sync.RWMutex
	id         string
	filePath   string
	entries    map[string]session.Entry // ID -> Entry lookup
	leafID     string                   // Current tip of the tree
	fileHandle *os.File
	labels     map[string]string // EntryID -> Current Label
	notify     func(string)
}

func (s *Session) ID() string     { return s.id }
func (s *Session) Path() string   { return s.filePath }
func (s *Session) LeafID() string { return s.leafID }

// Append persists a generic entry and advances the leaf pointer.
func (s *Session) Append(e session.Entry) error {
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

	if e.Type == session.TypeLabel && e.Label != nil {
		s.labels[e.Label.TargetID] = e.Label.Label
	}

	if s.notify != nil {
		s.notify(s.id)
	}

	return nil
}

func (s *Session) AppendMessage(role session.MessageRole, content []session.Content) (string, error) {
	id := uuid.New().String()
	e := session.Entry{
		Type: session.TypeMessage,
		ID:   id,
		Message: &session.MessageEntry{
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
	e := session.Entry{
		Type: session.TypeThinkingLevel,
		ID:   id,
		ThinkingLevel: &session.ThinkingLevelEntry{
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
	e := session.Entry{
		Type: session.TypeModelChange,
		ID:   id,
		ModelChange: &session.ModelChangeEntry{
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
	e := session.Entry{
		Type: session.TypeCompaction,
		ID:   id,
		Compaction: &session.CompactionEntry{
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
	e := session.Entry{
		Type: session.TypeSessionInfo,
		ID:   id,
		SessionInfo: &session.SessionInfoEntry{
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
	e := session.Entry{
		Type: session.TypeCustom,
		ID:   id,
		Custom: &session.CustomEntry{
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
	e := session.Entry{
		Type: session.TypeLabel,
		ID:   id,
		Label: &session.LabelEntry{
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
	e := session.Entry{
		Type: session.TypeBranchSummary,
		ID:   id,
		BranchSummary: &session.BranchSummaryEntry{
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
	dir := filepath.Dir(s.filePath)

	newS, err := NewManager(dir).New(s.id)
	if err != nil {
		return "", err
	}
	defer newS.Close()

	s.mu.RLock()
	var path []session.Entry
	currID := leafID
	for currID != "" {
		e, ok := s.entries[currID]
		if !ok {
			s.mu.RUnlock()
			return "", fmt.Errorf("broken path at %s", currID)
		}
		path = append([]session.Entry{e}, path...)
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

func (s *Session) GetContext() ([]session.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var fullPath []session.Entry
	currID := s.leafID

	for currID != "" {
		e, ok := s.entries[currID]
		if !ok {
			return nil, fmt.Errorf("broken parent link: %s", currID)
		}
		fullPath = append([]session.Entry{e}, fullPath...)

		if e.ParentID == nil {
			break
		}
		currID = *e.ParentID
	}

	var mostRecentCompaction *session.CompactionEntry
	compactionIndex := -1

	for i := len(fullPath) - 1; i >= 0; i-- {
		if fullPath[i].Type == session.TypeCompaction {
			mostRecentCompaction = fullPath[i].Compaction
			compactionIndex = i
			break
		}
	}

	if mostRecentCompaction == nil {
		return fullPath, nil
	}

	resolved := []session.Entry{fullPath[compactionIndex]}
	firstKeptID := mostRecentCompaction.FirstKeptEntryID
	include := false
	for _, e := range fullPath {
		if e.ID == firstKeptID {
			include = true
		}
		if include && e.Type != session.TypeCompaction {
			resolved = append(resolved, e)
		}
	}

	return resolved, nil
}

func (s *Session) GetTree() ([]session.TreeNode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	byParent := make(map[string][]session.Entry)
	var roots []session.Entry

	for _, e := range s.entries {
		if e.ParentID == nil {
			roots = append(roots, e)
		} else {
			byParent[*e.ParentID] = append(byParent[*e.ParentID], e)
		}
	}

	var build func(session.Entry) session.TreeNode
	build = func(e session.Entry) session.TreeNode {
		node := session.TreeNode{
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

	var tree []session.TreeNode
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
