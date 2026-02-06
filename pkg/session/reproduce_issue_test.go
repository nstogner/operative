package session_test

import (
	"os"
	"testing"

	"github.com/mariozechner/coding-agent/session/pkg/session"
	"github.com/mariozechner/coding-agent/session/pkg/session/jsonl"
)

func TestSession_AppendMultipleAssistantMessages(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session_repro")
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

	// 1. Append User Message
	msg1, err := s.AppendMessage(session.RoleUser, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "User Request"}}})
	if err != nil {
		t.Fatal(err)
	}

	// 2. Append Assistant Message 1
	msg2, err := s.AppendMessage(session.RoleAssistant, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Assistant Response 1"}}})
	if err != nil {
		t.Fatal(err)
	}

	// 3. Append Assistant Message 2
	msg3, err := s.AppendMessage(session.RoleAssistant, []session.Content{{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Assistant Response 2"}}})
	if err != nil {
		t.Fatal(err)
	}

	// 4. Verify Context
	ctx, err := s.GetContext()
	if err != nil {
		t.Fatal(err)
	}

	if len(ctx) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(ctx))
	}

	if ctx[0].ID != msg1 {
		t.Errorf("expected 1st message ID %s, got %s", msg1, ctx[0].ID)
	}
	if ctx[1].ID != msg2 {
		t.Errorf("expected 2nd message ID %s, got %s", msg2, ctx[1].ID)
	}
	if ctx[2].ID != msg3 {
		t.Errorf("expected 3rd message ID %s, got %s", msg3, ctx[2].ID)
	}

	// Check content
	if ctx[1].Message.Content[0].Text.Content != "Assistant Response 1" {
		t.Errorf("expected 'Assistant Response 1', got '%s'", ctx[1].Message.Content[0].Text.Content)
	}
	if ctx[2].Message.Content[0].Text.Content != "Assistant Response 2" {
		t.Errorf("expected 'Assistant Response 2', got '%s'", ctx[2].Message.Content[0].Text.Content)
	}
}
