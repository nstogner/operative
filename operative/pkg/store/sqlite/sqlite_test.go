package sqlite

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/nstogner/operative/pkg/domain"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	tmpFile := t.TempDir() + "/test.db"
	s, err := New(tmpFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		s.Close()
		os.Remove(tmpFile)
	})
	return s
}

func TestOperativeCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	op := &domain.Operative{
		ID:                "op-1",
		Name:              "Test Operative",
		AdminInstructions: "You are a test operative.",
		Model:             "gemini-2.0-flash",
	}

	// Create
	if err := s.Create(ctx, op); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Get
	got, err := s.Get(ctx, "op-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Test Operative" {
		t.Errorf("Name = %q, want %q", got.Name, "Test Operative")
	}

	// Update
	got.Name = "Updated Name"
	if err := s.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got2, _ := s.Get(ctx, "op-1")
	if got2.Name != "Updated Name" {
		t.Errorf("after update: Name = %q, want %q", got2.Name, "Updated Name")
	}

	// List
	ops, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ops) != 1 {
		t.Errorf("List len = %d, want 1", len(ops))
	}

	// Delete
	if err := s.Delete(ctx, "op-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.Get(ctx, "op-1")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestUpdateInstructions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Create(ctx, &domain.Operative{ID: "op-1", Name: "test"})

	if err := s.UpdateInstructions(ctx, "op-1", "new admin", "new operative"); err != nil {
		t.Fatalf("UpdateInstructions: %v", err)
	}

	got, _ := s.Get(ctx, "op-1")
	if got.AdminInstructions != "new admin" || got.OperativeInstructions != "new operative" {
		t.Errorf("instructions = (%q, %q), want (\"new admin\", \"new operative\")",
			got.AdminInstructions, got.OperativeInstructions)
	}
}

func TestStreamAppendAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Create(ctx, &domain.Operative{ID: "op-1", Name: "test"})

	// Append several entries
	for i := 0; i < 5; i++ {
		entry := &domain.StreamEntry{
			ID:          uuid.New().String(),
			OperativeID: "op-1",
			Role:        domain.RoleUser,
			ContentType: domain.ContentTypeText,
			Content:     "message " + string(rune('A'+i)),
		}
		if err := s.Append(ctx, entry); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	// Get all
	entries, err := s.GetEntries(ctx, "op-1", 0)
	if err != nil {
		t.Fatalf("GetEntries: %v", err)
	}
	if len(entries) != 5 {
		t.Errorf("GetEntries len = %d, want 5", len(entries))
	}

	// Get with limit
	limited, err := s.GetEntries(ctx, "op-1", 3)
	if err != nil {
		t.Fatalf("GetEntries limit: %v", err)
	}
	if len(limited) != 3 {
		t.Errorf("GetEntries limited len = %d, want 3", len(limited))
	}
	// Should be the last 3
	if limited[0].Content != entries[2].Content {
		t.Errorf("first limited entry = %q, want %q", limited[0].Content, entries[2].Content)
	}
}

func TestStreamGetEntriesAfter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Create(ctx, &domain.Operative{ID: "op-1", Name: "test"})

	var ids []string
	for i := 0; i < 5; i++ {
		id := uuid.New().String()
		ids = append(ids, id)
		s.Append(ctx, &domain.StreamEntry{
			ID:          id,
			OperativeID: "op-1",
			Role:        domain.RoleUser,
			ContentType: domain.ContentTypeText,
			Content:     "msg",
		})
	}

	// Get entries after the 3rd one
	after, err := s.GetEntriesAfter(ctx, "op-1", ids[2])
	if err != nil {
		t.Fatalf("GetEntriesAfter: %v", err)
	}
	if len(after) != 2 {
		t.Errorf("GetEntriesAfter len = %d, want 2", len(after))
	}
}

func TestStreamCompaction(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Create(ctx, &domain.Operative{ID: "op-1", Name: "test"})

	var ids []string
	for i := 0; i < 5; i++ {
		id := uuid.New().String()
		ids = append(ids, id)
		s.Append(ctx, &domain.StreamEntry{
			ID:          id,
			OperativeID: "op-1",
			Role:        domain.RoleUser,
			ContentType: domain.ContentTypeText,
			Content:     fmt.Sprintf("msg-%d", i),
		})
	}

	// Compact — this appends a compaction_summary entry (immutable, no deletion).
	if err := s.Compact(ctx, "op-1", "summary of first messages"); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	// Append 2 more entries after compaction.
	for i := 5; i < 7; i++ {
		s.Append(ctx, &domain.StreamEntry{
			ID:          uuid.New().String(),
			OperativeID: "op-1",
			Role:        domain.RoleUser,
			ContentType: domain.ContentTypeText,
			Content:     fmt.Sprintf("msg-%d", i),
		})
	}

	// GetEntries should return: compaction_summary + msg-5 + msg-6 = 3.
	entries, err := s.GetEntries(ctx, "op-1", 0)
	if err != nil {
		t.Fatalf("GetEntries: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("after compaction GetEntries len = %d, want 3", len(entries))
	}
	if entries[0].Role != domain.RoleCompactionSummary {
		t.Errorf("first entry role = %q, want %q", entries[0].Role, domain.RoleCompactionSummary)
	}
	if entries[0].Content != "summary of first messages" {
		t.Errorf("compaction content = %q, want %q", entries[0].Content, "summary of first messages")
	}

	// Verify original entries still exist in DB (immutability).
	var totalCount int
	s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM stream_entries WHERE operative_id=?`, "op-1",
	).Scan(&totalCount)
	// 5 original + 1 compaction + 2 after = 8
	if totalCount != 8 {
		t.Errorf("total entries in DB = %d, want 8 (immutable)", totalCount)
	}
}

func TestStreamSubscribe(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Create(ctx, &domain.Operative{ID: "op-1", Name: "test"})

	ch := s.Subscribe()

	s.Append(ctx, &domain.StreamEntry{
		ID:          uuid.New().String(),
		OperativeID: "op-1",
		Role:        domain.RoleUser,
		ContentType: domain.ContentTypeText,
		Content:     "hello",
	})

	select {
	case id := <-ch:
		if id != "op-1" {
			t.Errorf("subscriber got %q, want %q", id, "op-1")
		}
	default:
		t.Error("subscriber did not receive event")
	}
}

func TestNoteCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Create(ctx, &domain.Operative{ID: "op-1", Name: "test"})

	note := &domain.Note{
		ID:          "note-1",
		OperativeID: "op-1",
		Title:       "Test Note",
		Content:     "Some content here",
	}

	// Create
	if err := s.CreateNote(ctx, note); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	// Get
	got, err := s.GetNote(ctx, "note-1")
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if got.Title != "Test Note" {
		t.Errorf("Title = %q, want %q", got.Title, "Test Note")
	}

	// List
	notes, err := s.ListNotes(ctx, "op-1")
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 1 {
		t.Errorf("ListNotes len = %d, want 1", len(notes))
	}

	// Update
	got.Title = "Updated Title"
	if err := s.UpdateNote(ctx, got); err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}

	// Delete
	if err := s.DeleteNote(ctx, "note-1"); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
}

func TestKeywordSearch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Create(ctx, &domain.Operative{ID: "op-1", Name: "test"})

	s.CreateNote(ctx, &domain.Note{ID: "n1", OperativeID: "op-1", Title: "Go Programming", Content: "Concurrency patterns"})
	s.CreateNote(ctx, &domain.Note{ID: "n2", OperativeID: "op-1", Title: "Python Tips", Content: "List comprehensions"})
	s.CreateNote(ctx, &domain.Note{ID: "n3", OperativeID: "op-1", Title: "Rust Guide", Content: "Ownership model"})

	results, err := s.KeywordSearch(ctx, "op-1", "Go")
	if err != nil {
		t.Fatalf("KeywordSearch: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("KeywordSearch len = %d, want 1", len(results))
	}

	results2, _ := s.KeywordSearch(ctx, "op-1", "model")
	if len(results2) != 1 {
		t.Errorf("KeywordSearch 'model' len = %d, want 1", len(results2))
	}
}

func TestListIDs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Empty store should return nil/empty.
	ids, err := s.ListIDs(ctx)
	if err != nil {
		t.Fatalf("ListIDs on empty store: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ListIDs len = %d, want 0", len(ids))
	}

	// Create a few operatives.
	s.Create(ctx, &domain.Operative{ID: "op-1", Name: "Op1", Model: "m"})
	s.Create(ctx, &domain.Operative{ID: "op-2", Name: "Op2", Model: "m"})
	s.Create(ctx, &domain.Operative{ID: "op-3", Name: "Op3", Model: "m"})

	ids, err = s.ListIDs(ctx)
	if err != nil {
		t.Fatalf("ListIDs: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("ListIDs len = %d, want 3", len(ids))
	}

	// Verify all IDs are present.
	idSet := map[string]bool{}
	for _, id := range ids {
		idSet[id] = true
	}
	for _, expected := range []string{"op-1", "op-2", "op-3"} {
		if !idSet[expected] {
			t.Errorf("ListIDs missing expected ID %q", expected)
		}
	}
}
