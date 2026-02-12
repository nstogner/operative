package store

import (
	"context"

	"github.com/nstogner/operative/pkg/domain"
)

// OperativeStore manages the persistence of operative configurations.
type OperativeStore interface {
	// Create persists a new operative. The ID field must be set by the caller.
	Create(ctx context.Context, op *domain.Operative) error

	// Get retrieves an operative by its unique ID.
	// Returns an error if the operative does not exist.
	Get(ctx context.Context, id string) (*domain.Operative, error)

	// List returns all operatives, ordered by creation time descending.
	List(ctx context.Context) ([]domain.Operative, error)

	// Update persists changes to an existing operative.
	// Only non-zero fields are updated. The ID field identifies which operative to update.
	Update(ctx context.Context, op *domain.Operative) error

	// Delete removes an operative by ID. Associated stream entries and notes
	// should be cleaned up by the caller or via cascade.
	Delete(ctx context.Context, id string) error

	// UpdateInstructions updates both the admin-set and operative-set instructions
	// for the given operative. Either may be empty to leave unchanged.
	UpdateInstructions(ctx context.Context, id string, adminInstructions, operativeInstructions string) error
}

// StreamStore manages the append-only message stream for operatives.
// Stream entries are immutable — compaction works by appending a summary entry
// rather than deleting old entries. Query methods return the "compacted view"
// (entries from the most recent compaction entry onward).
type StreamStore interface {
	// Append adds a new entry to the end of the operative's stream.
	// The entry's ID and Timestamp should be set by the caller.
	Append(ctx context.Context, entry *domain.StreamEntry) error

	// GetEntries returns the compacted view of entries for an operative.
	// Only entries at or after the most recent compaction_summary entry are
	// returned, in chronological order. If limit > 0, returns at most that many.
	GetEntries(ctx context.Context, operativeID string, limit int) ([]domain.StreamEntry, error)

	// GetEntriesAfter returns entries appended after the given entry ID,
	// respecting the compacted view.
	GetEntriesAfter(ctx context.Context, operativeID string, afterID string) ([]domain.StreamEntry, error)

	// Compact appends a compaction_summary entry to the stream. Older entries
	// remain in the database but are excluded from GetEntries/GetEntriesAfter.
	Compact(ctx context.Context, operativeID string, summary string) error

	// Subscribe returns a channel that emits operative IDs whenever new entries
	// are appended to any operative's stream. Used by the controller to trigger
	// the next step in the control loop.
	Subscribe() <-chan string
}

// NoteStore manages persistent, searchable notes attached to operatives.
type NoteStore interface {
	// CreateNote persists a new note. The ID field must be set by the caller.
	CreateNote(ctx context.Context, note *domain.Note) error

	// GetNote retrieves a note by its unique ID.
	// Returns an error if the note does not exist.
	GetNote(ctx context.Context, id string) (*domain.Note, error)

	// ListNotes returns all notes for the given operative, ordered by creation time descending.
	ListNotes(ctx context.Context, operativeID string) ([]domain.Note, error)

	// UpdateNote persists changes to an existing note.
	UpdateNote(ctx context.Context, note *domain.Note) error

	// DeleteNote removes a note by ID.
	DeleteNote(ctx context.Context, id string) error

	// KeywordSearch returns notes whose title or content contain the given query string.
	// Results are ordered by relevance (simple substring match).
	KeywordSearch(ctx context.Context, operativeID string, query string) ([]domain.Note, error)

	// VectorSearch returns notes semantically similar to the given query.
	// This uses embedding-based similarity search.
	// Returns ErrNotImplemented if vector search is not configured.
	VectorSearch(ctx context.Context, operativeID string, query string) ([]domain.Note, error)
}
