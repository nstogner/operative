package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/nstogner/operative/pkg/domain"
	"github.com/nstogner/operative/pkg/store"
)

// Store implements OperativeStore, StreamStore, and NoteStore using SQLite.
type Store struct {
	db          *sql.DB
	subscribers []chan string
	mu          sync.RWMutex
}

// Verify interface compliance at compile time.
var _ store.OperativeStore = (*Store)(nil)
var _ store.StreamStore = (*Store)(nil)
var _ store.NoteStore = (*Store)(nil)

// ListIDs returns just the IDs of all operatives (used by sandbox reconciliation).
func (s *Store) ListIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM operatives`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// New opens (or creates) a SQLite database at the given path and runs migrations.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS operatives (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		admin_instructions TEXT NOT NULL DEFAULT '',
		operative_instructions TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		compaction_model TEXT NOT NULL DEFAULT '',
		compaction_threshold REAL NOT NULL DEFAULT 0.6,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS stream_entries (
		id TEXT PRIMARY KEY,
		operative_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content_type TEXT NOT NULL DEFAULT 'text',
		content TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		seq INTEGER NOT NULL,
		FOREIGN KEY (operative_id) REFERENCES operatives(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_stream_operative_seq ON stream_entries(operative_id, seq);

	CREATE TABLE IF NOT EXISTS notes (
		id TEXT PRIMARY KEY,
		operative_id TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (operative_id) REFERENCES operatives(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_notes_operative ON notes(operative_id);
	`
	_, err := s.db.Exec(schema)
	return err
}

// --- OperativeStore ---

func (s *Store) Create(ctx context.Context, op *domain.Operative) error {
	now := time.Now().UTC()
	op.CreatedAt = now
	op.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO operatives (id, name, admin_instructions, operative_instructions, model, compaction_model, compaction_threshold, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		op.ID, op.Name, op.AdminInstructions, op.OperativeInstructions,
		op.Model, op.CompactionModel, op.CompactionThreshold,
		op.CreatedAt, op.UpdatedAt,
	)
	return err
}

func (s *Store) Get(ctx context.Context, id string) (*domain.Operative, error) {
	op := &domain.Operative{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, admin_instructions, operative_instructions, model, compaction_model, compaction_threshold, created_at, updated_at
		 FROM operatives WHERE id = ?`, id,
	).Scan(&op.ID, &op.Name, &op.AdminInstructions, &op.OperativeInstructions,
		&op.Model, &op.CompactionModel, &op.CompactionThreshold,
		&op.CreatedAt, &op.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("operative not found: %s", id)
	}
	return op, err
}

func (s *Store) List(ctx context.Context) ([]domain.Operative, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, admin_instructions, operative_instructions, model, compaction_model, compaction_threshold, created_at, updated_at
		 FROM operatives ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []domain.Operative
	for rows.Next() {
		var op domain.Operative
		if err := rows.Scan(&op.ID, &op.Name, &op.AdminInstructions, &op.OperativeInstructions,
			&op.Model, &op.CompactionModel, &op.CompactionThreshold,
			&op.CreatedAt, &op.UpdatedAt,
		); err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func (s *Store) Update(ctx context.Context, op *domain.Operative) error {
	op.UpdatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE operatives SET name=?, admin_instructions=?, operative_instructions=?, model=?, compaction_model=?, compaction_threshold=?, updated_at=?
		 WHERE id=?`,
		op.Name, op.AdminInstructions, op.OperativeInstructions,
		op.Model, op.CompactionModel, op.CompactionThreshold,
		op.UpdatedAt, op.ID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("operative not found: %s", op.ID)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM operatives WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("operative not found: %s", id)
	}
	return nil
}

func (s *Store) UpdateInstructions(ctx context.Context, id string, adminInstructions, operativeInstructions string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE operatives SET admin_instructions=?, operative_instructions=?, updated_at=? WHERE id=?`,
		adminInstructions, operativeInstructions, time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("operative not found: %s", id)
	}
	return nil
}

// --- StreamStore ---

func (s *Store) Append(ctx context.Context, entry *domain.StreamEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	// Get next sequence number.
	var maxSeq int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM stream_entries WHERE operative_id=?`,
		entry.OperativeID,
	).Scan(&maxSeq)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO stream_entries (id, operative_id, role, content_type, content, model, timestamp, seq)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.OperativeID, entry.Role, entry.ContentType,
		entry.Content, entry.Model, entry.Timestamp, maxSeq+1,
	)
	if err != nil {
		return err
	}

	// Notify subscribers.
	s.notifySubscribers(entry.OperativeID)
	return nil
}

func (s *Store) GetEntries(ctx context.Context, operativeID string, limit int) ([]domain.StreamEntry, error) {
	// Find the seq of the last compaction_summary entry (if any).
	var compactionSeq int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM stream_entries WHERE operative_id=? AND role=?`,
		operativeID, domain.RoleCompactionSummary,
	).Scan(&compactionSeq)
	if err != nil {
		return nil, err
	}

	query := `SELECT id, operative_id, role, content_type, content, model, timestamp
		FROM stream_entries WHERE operative_id=? AND seq >= ? ORDER BY seq ASC`
	var args []any
	args = append(args, operativeID, compactionSeq)

	if limit > 0 {
		// Subquery to get only the last N entries (from the compacted view) in ASC order.
		query = `SELECT id, operative_id, role, content_type, content, model, timestamp FROM (
			SELECT id, operative_id, role, content_type, content, model, timestamp, seq
			FROM stream_entries WHERE operative_id=? AND seq >= ? ORDER BY seq DESC LIMIT ?
		) sub ORDER BY seq ASC`
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []domain.StreamEntry
	for rows.Next() {
		var e domain.StreamEntry
		if err := rows.Scan(&e.ID, &e.OperativeID, &e.Role, &e.ContentType, &e.Content, &e.Model, &e.Timestamp); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *Store) GetEntriesAfter(ctx context.Context, operativeID string, afterID string) ([]domain.StreamEntry, error) {
	// Find the seq of the afterID entry.
	var afterSeq int
	err := s.db.QueryRowContext(ctx,
		`SELECT seq FROM stream_entries WHERE id=? AND operative_id=?`, afterID, operativeID,
	).Scan(&afterSeq)
	if err == sql.ErrNoRows {
		// If the afterID doesn't exist, return all entries.
		return s.GetEntries(ctx, operativeID, 0)
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, operative_id, role, content_type, content, model, timestamp
		 FROM stream_entries WHERE operative_id=? AND seq > ? ORDER BY seq ASC`,
		operativeID, afterSeq,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []domain.StreamEntry
	for rows.Next() {
		var e domain.StreamEntry
		if err := rows.Scan(&e.ID, &e.OperativeID, &e.Role, &e.ContentType, &e.Content, &e.Model, &e.Timestamp); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *Store) Compact(ctx context.Context, operativeID string, summary string) error {
	// Append a compaction summary entry. GetEntries will use this as the new
	// starting point, effectively hiding all older entries from the view.
	return s.Append(ctx, &domain.StreamEntry{
		ID:          fmt.Sprintf("compaction-%d", time.Now().UnixNano()),
		OperativeID: operativeID,
		Role:        domain.RoleCompactionSummary,
		ContentType: domain.ContentTypeText,
		Content:     summary,
	})
}

func (s *Store) Subscribe() <-chan string {
	ch := make(chan string, 64)
	s.mu.Lock()
	s.subscribers = append(s.subscribers, ch)
	s.mu.Unlock()
	return ch
}

func (s *Store) notifySubscribers(operativeID string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ch := range s.subscribers {
		select {
		case ch <- operativeID:
		default:
			// Drop if subscriber is not consuming fast enough.
		}
	}
}

// --- NoteStore ---

func (s *Store) CreateNote(ctx context.Context, note *domain.Note) error {
	now := time.Now().UTC()
	note.CreatedAt = now
	note.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notes (id, operative_id, title, content, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		note.ID, note.OperativeID, note.Title, note.Content, note.CreatedAt, note.UpdatedAt,
	)
	return err
}

func (s *Store) GetNote(ctx context.Context, id string) (*domain.Note, error) {
	note := &domain.Note{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, operative_id, title, content, created_at, updated_at FROM notes WHERE id=?`, id,
	).Scan(&note.ID, &note.OperativeID, &note.Title, &note.Content, &note.CreatedAt, &note.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("note not found: %s", id)
	}
	return note, err
}

func (s *Store) ListNotes(ctx context.Context, operativeID string) ([]domain.Note, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, operative_id, title, content, created_at, updated_at
		 FROM notes WHERE operative_id=? ORDER BY created_at DESC`, operativeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []domain.Note
	for rows.Next() {
		var n domain.Note
		if err := rows.Scan(&n.ID, &n.OperativeID, &n.Title, &n.Content, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

func (s *Store) UpdateNote(ctx context.Context, note *domain.Note) error {
	note.UpdatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE notes SET title=?, content=?, updated_at=? WHERE id=?`,
		note.Title, note.Content, note.UpdatedAt, note.ID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("note not found: %s", note.ID)
	}
	return nil
}

func (s *Store) DeleteNote(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM notes WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("note not found: %s", id)
	}
	return nil
}

func (s *Store) KeywordSearch(ctx context.Context, operativeID string, query string) ([]domain.Note, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, operative_id, title, content, created_at, updated_at
		 FROM notes WHERE operative_id=? AND (title LIKE '%' || ? || '%' OR content LIKE '%' || ? || '%')
		 ORDER BY created_at DESC`,
		operativeID, query, query,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []domain.Note
	for rows.Next() {
		var n domain.Note
		if err := rows.Scan(&n.ID, &n.OperativeID, &n.Title, &n.Content, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

func (s *Store) VectorSearch(ctx context.Context, operativeID string, query string) ([]domain.Note, error) {
	return nil, fmt.Errorf("vector search not implemented: requires embedding model configuration")
}
