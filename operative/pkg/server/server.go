package server

import (
	"context"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/nstogner/operative/pkg/model"
	"github.com/nstogner/operative/pkg/sandbox"
	"github.com/nstogner/operative/pkg/store"
)

// Server serves the web UI and REST API for the operative system.
type Server struct {
	operatives store.OperativeStore
	stream     store.StreamStore
	notes      store.NoteStore
	provider   model.Provider
	sandbox    sandbox.Manager
	distFS     embed.FS
	srv        *http.Server
}

// New creates a new Server.
func New(
	operatives store.OperativeStore,
	stream store.StreamStore,
	notes store.NoteStore,
	provider model.Provider,
	sandbox sandbox.Manager,
	distFS embed.FS,
) *Server {
	return &Server{
		operatives: operatives,
		stream:     stream,
		notes:      notes,
		provider:   provider,
		sandbox:    sandbox,
		distFS:     distFS,
	}
}

// Start starts the HTTP server.
func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	// Operative routes
	mux.HandleFunc("GET /api/operatives", s.handleListOperatives)
	mux.HandleFunc("POST /api/operatives", s.handleCreateOperative)
	mux.HandleFunc("GET /api/operatives/{id}", s.handleGetOperative)
	mux.HandleFunc("PUT /api/operatives/{id}", s.handleUpdateOperative)
	mux.HandleFunc("DELETE /api/operatives/{id}", s.handleDeleteOperative)

	// Stream
	mux.HandleFunc("GET /api/operatives/{id}/stream", s.handleGetStream)

	// Notes
	mux.HandleFunc("GET /api/operatives/{id}/notes", s.handleListNotes)
	mux.HandleFunc("POST /api/operatives/{id}/notes", s.handleCreateNote)
	mux.HandleFunc("GET /api/operatives/{id}/notes/keyword-search", s.handleKeywordSearchNotes)
	mux.HandleFunc("GET /api/operatives/{id}/notes/vector-search", s.handleVectorSearchNotes)
	mux.HandleFunc("GET /api/notes/{id}", s.handleGetNote)
	mux.HandleFunc("PUT /api/notes/{id}", s.handleUpdateNote)
	mux.HandleFunc("DELETE /api/notes/{id}", s.handleDeleteNote)

	// Sandbox
	mux.HandleFunc("GET /api/operatives/{id}/sandbox/status", s.handleSandboxStatus)

	// Models
	mux.HandleFunc("GET /api/models", s.handleListModels)

	// WebSocket
	mux.HandleFunc("/api/operatives/{id}/chat", s.handleChatWebSocket)

	// Static assets (SPA fallback)
	mux.HandleFunc("/", s.handleStatic)

	s.srv = &http.Server{
		Addr:    addr,
		Handler: s.corsMiddleware(mux),
	}

	slog.Info("Starting web server", "addr", addr)
	return s.srv.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
		http.NotFound(w, r)
		return
	}

	path := r.URL.Path
	if path == "/" {
		path = "index.html"
	} else if path[0] == '/' {
		path = path[1:]
	}

	distFS, err := fs.Sub(s.distFS, "dist")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Try serving the exact file.
	f, err := distFS.Open(path)
	if err == nil {
		defer f.Close()
		stat, _ := f.Stat()
		if !stat.IsDir() {
			http.FileServer(http.FS(distFS)).ServeHTTP(w, r)
			return
		}
	}

	// Fallback to index.html for SPA routing.
	index, err := distFS.Open("index.html")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer index.Close()
	http.ServeContent(w, r, "index.html", time.Time{}, index.(io.ReadSeeker))
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
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
