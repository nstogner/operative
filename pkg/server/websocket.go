package server

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"strings"

	"github.com/gorilla/websocket"
	"github.com/mariozechner/coding-agent/session/pkg/store"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all for now (Dev/Prod separation handled elsewhere or allow local)
	},
}

func (s *Server) handleChatWebSocket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "Missing session ID", http.StatusBadRequest)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("Failed to upgrade websocket", "error", err)
		return
	}
	defer ws.Close()

	sess, err := s.manager.LoadSession(id)
	if err != nil {
		slog.Error("Failed to load session", "id", id, "error", err)
		ws.WriteJSON(map[string]string{"error": "Session not found"})
		return
	}
	defer sess.Close()

	// Channel to signal connection close
	done := make(chan struct{})

	// Subscribe to updates
	updates := s.manager.Subscribe()

	// Track sent message IDs to avoid duplicates
	sentIDs := make(map[string]bool)
	// Pre-fill sent entries
	// Actually, on connect, we should probably send full history or let client fetch it via REST.
	// Let's assume client fetches history via REST separately or expects it here.
	// For simplicity, let's just stream NEW updates?
	// Or simpler: Stream EVERYTHING on connect, then updates.

	// Initial Sync
	if err := s.syncSession(ws, sess, sentIDs); err != nil {
		slog.Error("Failed initial sync", "error", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)

	// Writer Loop (Pusher)
	go func() {
		defer wg.Done()
		defer ws.Close()

		ticker := time.NewTicker(500 * time.Millisecond) // Heartbeat/Poll backup
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case eventID := <-updates:
				if eventID == id {
					if err := s.syncSession(ws, sess, sentIDs); err != nil {
						slog.Error("Failed (re)sync", "error", err)
						return
					}
				}
			case <-ticker.C:
				// Keepalive / check
			}
		}
	}()

	// Reader Loop
	for {
		// Read message (User Input)
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

		// Append to session
		if msg.Content != "" {
			if strings.HasPrefix(msg.Content, "/") {
				// Handle commands if needed, or pass through as text
			}

			sess.AppendMessage(store.RoleUser, []store.Content{
				{
					Type: store.ContentTypeText,
					Text: &store.TextContent{Content: msg.Content},
				},
			})
			// Append triggers event -> Writer Loop picks it up
		}
	}

	close(done)
	wg.Wait()
}

func (s *Server) syncSession(ws *websocket.Conn, sess store.Session, sentIDs map[string]bool) error {
	entries, err := sess.GetContext()
	if err != nil {
		return err
	}

	// Naively send any entry we haven't tracked yet
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
