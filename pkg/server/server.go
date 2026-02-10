package server

import (
	"embed"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/runner"
	"github.com/mariozechner/coding-agent/session/pkg/store"
)

// Server serves the web UI and API.
type Server struct {
	manager  store.Manager
	runner   *runner.Runner
	provider models.ModelProvider
	distFS   embed.FS
	srv      *http.Server
	agents   store.Manager // Using manager for agents for now
}

// New creates a new Server.
func New(manager store.Manager, provider models.ModelProvider, distFS embed.FS) *Server {
	return &Server{
		manager:  manager,
		provider: provider,
		distFS:   distFS,
	}
}

// Start starts the HTTP server.
func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	// API Routes
	mux.HandleFunc("GET /api/agents", s.handleListAgents)
	mux.HandleFunc("GET /api/agents/{id}", s.handleGetAgent)
	mux.HandleFunc("POST /api/agents", s.handleCreateUpdateAgent)
	mux.HandleFunc("DELETE /api/agents/{id}", s.handleDeleteAgent)

	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleGetSession)

	// Models
	mux.HandleFunc("GET /api/models", s.handleListModels)

	// WebSocket

	// WebSocket
	mux.HandleFunc("/api/sessions/{id}/chat", s.handleChatWebSocket)

	// Static Assets
	// TODO: Implement proper static file serving with fallback to index.html
	// For now, let's keep it simple or use a middleware.

	s.srv = &http.Server{
		Addr:    addr,
		Handler: s.corsMiddleware(mux),
	}

	slog.Info("Starting web server", "addr", addr)
	return s.srv.ListenAndServe()
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify origin in prod, allow all in dev
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) errorResponse(w http.ResponseWriter, status int, err error) {
	slog.Error("API Error", "error", err)
	s.jsonResponse(w, status, map[string]string{"error": err.Error()})
}
