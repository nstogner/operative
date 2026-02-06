package session_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mariozechner/coding-agent/session/pkg/session"
	"github.com/mariozechner/coding-agent/session/pkg/session/jsonl"
)

func TestSession_AppendAndContext(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m := jsonl.NewManager(tempDir)
	s, err := m.New("")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// 1. Append messages
	msg1, err := s.AppendMessage(session.RoleUser, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	msg2, err := s.AppendMessage(session.RoleAssistant, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Hi"}}})
	if err != nil {
		t.Fatal(err)
	}

	// 2. Check context
	ctx, err := s.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx) != 2 {
		t.Errorf("expected 2 messages, got %d", len(ctx))
	}
	if ctx[0].ID != msg1 || ctx[1].ID != msg2 {
		t.Error("context order or IDs mismatch")
	}

	// 3. Branching
	err = s.Branch(msg1)
	if err != nil {
		t.Fatal(err)
	}
	msg3, err := s.AppendMessage(session.RoleUser, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "New branch"}}})
	if err != nil {
		t.Fatal(err)
	}

	ctx, err = s.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx) != 2 {
		t.Errorf("expected 2 messages in branch, got %d", len(ctx))
	}
	if ctx[0].ID != msg1 || ctx[1].ID != msg3 {
		t.Error("branch context mismatch")
	}

	// 4. Compaction
	_, err = s.AppendCompaction("Summary", msg3, 100)
	if err != nil {
		t.Fatal(err)
	}
	msg4, err := s.AppendMessage(session.RoleAssistant, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "After compaction"}}})
	if err != nil {
		t.Fatal(err)
	}

	ctx, err = s.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx) != 3 {
		t.Errorf("expected 3 entries after compaction, got %d", len(ctx))
	}
	if ctx[0].Type != session.TypeCompaction || ctx[1].ID != msg3 || ctx[2].ID != msg4 {
		t.Error("compaction context resolution mismatch")
	}

	printJSONLFiles(t, tempDir)
}

func TestSession_Persistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session_persist")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m := jsonl.NewManager(tempDir)
	s, err := m.New("")
	if err != nil {
		t.Fatal(err)
	}
	msg1, _ := s.AppendMessage(session.RoleUser, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Store me"}}})
	id := s.ID()
	s.Close()

	// Reload
	s2, err := m.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	if s2.LeafID() != msg1 {
		t.Errorf("leafID not restored, got %s, want %s", s2.LeafID(), msg1)
	}

	printJSONLFiles(t, tempDir)
}

func TestSession_MetadataChanges(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session_meta")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m := jsonl.NewManager(tempDir)
	s, err := m.New("")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.AppendThinkingLevelChange("high")
	s.AppendModelChange("openai", "gpt-4o")
	s.AppendMessage(session.RoleUser, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Configured?"}}})

	ctx, err := s.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx) != 3 {
		t.Errorf("expected 3 entries, got %d", len(ctx))
	}

	printJSONLFiles(t, tempDir)
}

func TestSession_LabelsAndTree(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session_labels")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m := jsonl.NewManager(tempDir)
	s, err := m.New("")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	id1, _ := s.AppendMessage(session.RoleUser, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "One"}}})
	s.SetLabel(id1, "start")
	s.AppendMessage(session.RoleAssistant, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Two"}}})

	tree, err := s.GetTree()
	if err != nil {
		t.Fatal(err)
	}

	if len(tree) != 1 || tree[0].Label != "start" {
		t.Errorf("tree structure or label missing, got %+v", tree)
	}

	printJSONLFiles(t, tempDir)
}

func TestSession_BranchingAdvanced(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session_branching_adv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m := jsonl.NewManager(tempDir)
	s, err := m.New("")
	if err != nil {
		t.Fatal(err)
	}

	id1, _ := s.AppendMessage(session.RoleUser, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Root"}}})
	s.AppendMessage(session.RoleAssistant, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Path A"}}})

	// Branch with summary
	idSummary, err := s.BranchWithSummary(id1, "Summarizing Path A")
	if err != nil {
		t.Fatal(err)
	}

	if s.LeafID() != idSummary {
		t.Errorf("leafID not updated to summary, got %s", s.LeafID())
	}

	// Create branched session
	newSessionPath, err := s.CreateBranchedSession(id1)
	if err != nil {
		t.Fatal(err)
	}
	if newSessionPath == "" {
		t.Error("branched session path empty")
	}

	printJSONLFiles(t, tempDir)
}

func TestManager_Extended(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "manager_ext")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m := jsonl.NewManager(tempDir)
	s1, _ := m.New("")
	s1.AppendMessage(session.RoleUser, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Source"}}})
	id1 := s1.ID()
	s1.Close()

	// Fork
	s2, err := m.ForkFrom(id1)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	if s2.ID() == id1 {
		t.Error("forked session should have new ID")
	}

	// List
	list, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 2 {
		t.Errorf("expected at least 2 sessions, got %d", len(list))
	}

	// ContinueRecent
	sRecent, err := m.ContinueRecent()
	if err != nil {
		t.Fatal(err)
	}
	defer sRecent.Close()
	if sRecent.ID() != s2.ID() {
		t.Errorf("ContinueRecent should return s2, got %s", sRecent.ID())
	}

	printJSONLFiles(t, tempDir)
}

func TestSession_CustomEntries(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session_custom")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m := jsonl.NewManager(tempDir)
	s, _ := m.New("")
	defer s.Close()

	data := map[string]any{"key": "value", "count": 42.0} // encoding/json decodes numbers as float64
	_, err = s.AppendCustomEntry("my-ext", data)
	if err != nil {
		t.Fatal(err)
	}

	tree, _ := s.GetTree()
	custom := tree[0].Entry.Custom
	if custom.CustomType != "my-ext" || custom.Data["key"] != "value" {
		t.Errorf("custom entry mismatch: %+v", custom)
	}

	printJSONLFiles(t, tempDir)
}

func TestSession_Miscellaneous(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session_misc")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m := jsonl.NewManager(tempDir)
	s, err := m.New("")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Test Path()
	if s.Path() == "" {
		t.Error("Path() returned empty string")
	}
	if !filepath.IsAbs(s.Path()) {
		t.Errorf("Path() should be absolute, got %s", s.Path())
	}

	// Test AppendSessionInfo()
	nameID, err := s.AppendSessionInfo("My Test Session")
	if err != nil {
		t.Fatalf("AppendSessionInfo failed: %v", err)
	}
	if nameID == "" {
		t.Error("AppendSessionInfo returned empty ID")
	}

	// Test Append() directly
	directID := "direct-id-123"
	err = s.Append(session.Entry{
		ID:   directID,
		Type: session.TypeMessage,
		Message: &session.MessageEntry{
			Role:    session.RoleUser,
			Content: []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Direct append"}}},
		},
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if s.LeafID() != directID {
		t.Errorf("LeafID should be %s, got %s", directID, s.LeafID())
	}

	ctx, err := s.GetContext()
	if err != nil {
		t.Fatal(err)
	}

	foundInfo := false
	foundDirect := false
	for _, e := range ctx {
		if e.Type == session.TypeSessionInfo && e.SessionInfo.Name == "My Test Session" {
			foundInfo = true
		}
		if e.ID == directID {
			foundDirect = true
		}
	}

	if !foundInfo {
		t.Error("SessionInfo not found in context")
	}
	if !foundDirect {
		t.Error("Directly appended entry not found in context")
	}

	printJSONLFiles(t, tempDir)
}

func printJSONLFiles(t *testing.T, dir string) {

	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	for _, f := range files {
		fmt.Printf("\n--- File: %s ---\n", filepath.Base(f))
		content, _ := os.ReadFile(f)
		fmt.Println(string(content))
		fmt.Println("-----------------")
	}
}
