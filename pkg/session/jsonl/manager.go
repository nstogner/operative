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
	"github.com/mariozechner/coding-agent/session/pkg/session"
)

// Manager implements the session.Manager interface using JSONL files.
type Manager struct {
	dir       string
	eventChan chan string
	mu        sync.RWMutex
	subs      []chan string
}

func (m *Manager) SetStatus(id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	indexPath := filepath.Join(m.dir, "index.json")
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

func NewManager(dir string) *Manager {
	m := &Manager{
		dir:       dir,
		eventChan: make(chan string, 100),
	}
	go m.broadcastLoop()
	return m
}

// Index represents the index.json structure
type Index struct {
	Sessions []SessionMeta `json:"sessions"`
}

type SessionMeta struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Status   string    `json:"status"`
	Created  time.Time `json:"created"`
	Modified time.Time `json:"modified"`
}

func (m *Manager) updateIndex(meta SessionMeta) error {
	indexPath := filepath.Join(m.dir, "index.json")

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
	indexPath := filepath.Join(m.dir, "index.json")
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

func (m *Manager) New(parentSessionID string) (session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	id := uuid.New().String()
	path := filepath.Join(m.dir, id+".jsonl")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create session file: %w", err)
	}

	s := &Session{
		id:         id,
		filePath:   path,
		entries:    make(map[string]session.Entry),
		fileHandle: f,
		labels:     make(map[string]string),
		notify:     m.publish,
	}

	now := time.Now()
	header := session.Header{
		Type:          session.TypeSession,
		ID:            id,
		Version:       1,
		ParentSession: parentSessionID,
		CreatedAt:     now,
	}

	if err := s.writeLine(header); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to write session header: %w", err)
	}

	// Update Index
	meta := SessionMeta{
		ID:       id,
		Path:     path,
		Status:   session.SessionStatusActive,
		Created:  now,
		Modified: now,
	}
	if err := m.updateIndex(meta); err != nil {
		slog.Error("Failed to update session index", "error", err)
		// Proceed anyway? Or fail? proceeding is safer for now but logging error
	}

	return s, nil
}

func (m *Manager) Load(id string) (session.Session, error) {
	// No lock needed for simple file open, but if we updated index on load (last accessed), we would need it.
	// For now, simple load.

	path := filepath.Join(m.dir, id+".jsonl")
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open session file: %w", err)
	}

	s := &Session{
		filePath:   path,
		entries:    make(map[string]session.Entry),
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

func (m *Manager) ContinueRecent() (session.Session, error) {
	infos, err := m.List()
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		return nil, fmt.Errorf("no sessions found in %s", m.dir)
	}
	return m.Load(infos[0].ID)
}

func (m *Manager) ForkFrom(id string) (session.Session, error) {
	source, err := m.Load(id)
	if err != nil {
		return nil, err
	}
	defer source.Close()

	dest, err := m.New(source.ID())
	if err != nil {
		return nil, err
	}

	// We need to access the hidden fileHandle of the source session.
	// Since both are in the same package (jsonl), we can do this if we cast.
	jsonlSource := source.(*Session)

	if _, err := jsonlSource.fileHandle.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(jsonlSource.fileHandle)
	scanner.Scan() // skip header

	for scanner.Scan() {
		var e session.Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if err := dest.Append(e); err != nil {
			dest.Close()
			return nil, err
		}
	}

	return dest, nil
}

func (m *Manager) List() ([]session.SessionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metas, err := m.readIndex()
	if err != nil {
		return nil, err
	}

	var infos []session.SessionInfo
	for _, meta := range metas {
		infos = append(infos, session.SessionInfo{
			ID:       meta.ID,
			Path:     meta.Path,
			Status:   meta.Status,
			Created:  meta.Created,
			Modified: meta.Modified,
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Modified.After(infos[j].Modified)
	})

	return infos, nil
}

func (m *Manager) getSessionInfo(path string) (session.SessionInfo, error) {
	// Deprecated in favor of index.json, but kept if needed by other internal logic (not used by List anymore)
	stat, err := os.Stat(path)
	if err != nil {
		return session.SessionInfo{}, err
	}

	f, err := os.Open(path)
	if err != nil {
		return session.SessionInfo{}, err
	}
	defer f.Close()

	var header session.Header
	if err := json.NewDecoder(f).Decode(&header); err != nil {
		return session.SessionInfo{}, err
	}

	return session.SessionInfo{
		ID:       header.ID,
		Path:     path,
		Created:  header.CreatedAt,
		Modified: stat.ModTime(),
	}, nil
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
		var h session.Header
		if err := json.Unmarshal(scanner.Bytes(), &h); err != nil {
			return fmt.Errorf("failed to unmarshal header: %w", err)
		}
		s.id = h.ID
	}

	for scanner.Scan() {
		var e session.Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		s.entries[e.ID] = e
		lastID = e.ID

		if e.Type == session.TypeLabel && e.Label != nil {
			s.labels[e.Label.TargetID] = e.Label.Label
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	s.leafID = lastID
	return nil
}
