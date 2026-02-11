package server

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

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

	// Session Actions
	mux.HandleFunc("POST /api/sessions/{id}/stop", s.handleStopSession)

	// WebSocket

	// WebSocket
	mux.HandleFunc("/api/sessions/{id}/chat", s.handleChatWebSocket)

	// Static Assets
	// Serve static files with fallback to index.html for SPA
	mux.HandleFunc("/", s.handleStatic)

	s.srv = &http.Server{
		Addr:    addr,
		Handler: s.corsMiddleware(mux),
	}

	slog.Info("Starting web server", "addr", addr)
	return s.srv.ListenAndServe()
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	// If it's an API request that wasn't matched, return 404
	// (Though specific API routes are handled by exact matches,
	// this captures /api/unknown)
	if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
		http.NotFound(w, r)
		return
	}

	// Try to serve existing file
	path := r.URL.Path
	if path == "/" {
		path = "index.html"
	} else if path[0] == '/' {
		path = path[1:]
	}

	// Check if file exists in embedded FS
	f, err := s.distFS.Open("dist/" + path)
	if err == nil {
		defer f.Close()
		// Determine content type (simple version)
		// ext := filepath.Ext(path)
		// ...
		// Better: use http.FileServer if possible, but we are inside a custom handler.
		// Let's use http.FileServer for the dist folder, but we need to strip prefix?.
		// Actually, let's just use http.ServeFile from FS if we can.
		// Since we are using embed.FS, we can use http.FS to create a file system.
	}

	// Refined approach:

	// Create a file server for "dist" directory
	distFS, err := fs.Sub(s.distFS, "dist")
	if err != nil {
		slog.Error("Failed to verify distfs", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check if file exists
	f, err = distFS.Open(path)
	if err == nil {
		defer f.Close()
		stat, _ := f.Stat()
		if !stat.IsDir() {
			http.FileServer(http.FS(distFS)).ServeHTTP(w, r)
			return
		}
	}

	// Fallback to index.html
	index, err := distFS.Open("index.html")
	if err != nil {
		slog.Error("Failed to open index.html", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer index.Close()

	// We need to serve index.html.
	// http.ServeContent requires ReadSeeker which embed.File implements.
	http.ServeContent(w, r, "index.html", time.Time{}, index.(io.ReadSeeker))
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
