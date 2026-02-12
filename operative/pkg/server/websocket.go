package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/nstogner/operative/pkg/domain"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (s *Server) handleChatWebSocket(w http.ResponseWriter, r *http.Request) {
	operativeID := r.PathValue("id")
	if operativeID == "" {
		http.Error(w, "Missing operative ID", http.StatusBadRequest)
		return
	}

	// Verify the operative exists.
	if _, err := s.operatives.Get(r.Context(), operativeID); err != nil {
		http.Error(w, "Operative not found", http.StatusNotFound)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("Failed to upgrade websocket", "error", err)
		return
	}
	defer ws.Close()

	done := make(chan struct{})
	updates := s.stream.Subscribe()

	// Send initial stream state.
	sentIDs := make(map[string]bool)
	if err := s.syncStream(ws, operativeID, sentIDs); err != nil {
		slog.Error("Failed initial stream sync", "error", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)

	// Writer goroutine: pushes new entries to the client.
	go func() {
		defer wg.Done()
		defer ws.Close()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case eventID := <-updates:
				if eventID == operativeID {
					if err := s.syncStream(ws, operativeID, sentIDs); err != nil {
						slog.Error("Failed stream sync", "error", err)
						return
					}
				}
			case <-ticker.C:
				// Keepalive
			}
		}
	}()

	// Reader loop: receives user messages.
	for {
		var msg struct {
			Content string `json:"content"`
		}
		if err := ws.ReadJSON(&msg); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				break
			}
			slog.Error("WebSocket read error", "error", err)
			break
		}

		if msg.Content != "" {
			entry := &domain.StreamEntry{
				ID:          uuid.New().String(),
				OperativeID: operativeID,
				Role:        domain.RoleUser,
				ContentType: domain.ContentTypeText,
				Content:     msg.Content,
			}
			if err := s.stream.Append(r.Context(), entry); err != nil {
				slog.Error("Failed to append user message", "error", err)
			}
		}
	}

	close(done)
	wg.Wait()
}

func (s *Server) syncStream(ws *websocket.Conn, operativeID string, sentIDs map[string]bool) error {
	entries, err := s.stream.GetEntries(context.Background(), operativeID, 0)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if !sentIDs[e.ID] {
			if err := ws.WriteJSON(e); err != nil {
				return err
			}
			sentIDs[e.ID] = true
		}
	}
	return nil
}
