package server

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/nstogner/operative/pkg/domain"
)

// --- Operatives ---

func (s *Server) handleListOperatives(w http.ResponseWriter, r *http.Request) {
	ops, err := s.operatives.List(r.Context())
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, ops)
}

func (s *Server) handleCreateOperative(w http.ResponseWriter, r *http.Request) {
	var op domain.Operative
	if err := json.NewDecoder(r.Body).Decode(&op); err != nil {
		s.errorResponse(w, http.StatusBadRequest, err)
		return
	}
	if op.ID == "" {
		op.ID = uuid.New().String()
	}
	if op.CompactionThreshold == 0 {
		op.CompactionThreshold = 0.6
	}
	if err := s.operatives.Create(r.Context(), &op); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	s.jsonResponse(w, http.StatusCreated, op)
}

func (s *Server) handleGetOperative(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	op, err := s.operatives.Get(r.Context(), id)
	if err != nil {
		s.errorResponse(w, http.StatusNotFound, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, op)
}

func (s *Server) handleUpdateOperative(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var op domain.Operative
	if err := json.NewDecoder(r.Body).Decode(&op); err != nil {
		s.errorResponse(w, http.StatusBadRequest, err)
		return
	}
	op.ID = id
	if err := s.operatives.Update(r.Context(), &op); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, op)
}

func (s *Server) handleDeleteOperative(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.operatives.Delete(r.Context(), id); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Stream ---

func (s *Server) handleGetStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	entries, err := s.stream.GetEntries(r.Context(), id, 0)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, entries)
}

// --- Notes ---

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	notes, err := s.notes.ListNotes(r.Context(), id)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, notes)
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	operativeID := r.PathValue("id")
	var note domain.Note
	if err := json.NewDecoder(r.Body).Decode(&note); err != nil {
		s.errorResponse(w, http.StatusBadRequest, err)
		return
	}
	note.OperativeID = operativeID
	if note.ID == "" {
		note.ID = uuid.New().String()
	}
	if err := s.notes.CreateNote(r.Context(), &note); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	s.jsonResponse(w, http.StatusCreated, note)
}

func (s *Server) handleGetNote(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	note, err := s.notes.GetNote(r.Context(), id)
	if err != nil {
		s.errorResponse(w, http.StatusNotFound, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, note)
}

func (s *Server) handleUpdateNote(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var note domain.Note
	if err := json.NewDecoder(r.Body).Decode(&note); err != nil {
		s.errorResponse(w, http.StatusBadRequest, err)
		return
	}
	note.ID = id
	if err := s.notes.UpdateNote(r.Context(), &note); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, note)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.notes.DeleteNote(r.Context(), id); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleKeywordSearchNotes(w http.ResponseWriter, r *http.Request) {
	operativeID := r.PathValue("id")
	query := r.URL.Query().Get("q")
	notes, err := s.notes.KeywordSearch(r.Context(), operativeID, query)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, notes)
}

func (s *Server) handleVectorSearchNotes(w http.ResponseWriter, r *http.Request) {
	operativeID := r.PathValue("id")
	query := r.URL.Query().Get("q")
	notes, err := s.notes.VectorSearch(r.Context(), operativeID, query)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, notes)
}

// --- Sandbox ---

func (s *Server) handleSandboxStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	status, err := s.sandbox.Status(r.Context(), id)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, map[string]string{"status": status})
}

// --- Models ---

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.provider.List(r.Context())
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, models)
}
