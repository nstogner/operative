package server

import (
	"encoding/json"
	"net/http"

	"github.com/mariozechner/coding-agent/session/pkg/store"
)

// --- Agents ---

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.manager.ListAgents()
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, agents)
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, err := s.manager.GetAgent(id)
	if err != nil {
		s.errorResponse(w, http.StatusNotFound, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, agent)
}

func (s *Server) handleCreateUpdateAgent(w http.ResponseWriter, r *http.Request) {
	var agent store.Agent
	if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
		s.errorResponse(w, http.StatusBadRequest, err)
		return
	}

	// If ID exists, it's an update? Or we assume POST can be both?
	// The manager handles logic.
	// We should probably check if we are updating or creating based on ID presence?
	// Manager.NewAgent generates ID if empty. UpdateAgent requires ID.

	if agent.ID == "" {
		if err := s.manager.NewAgent(&agent); err != nil {
			s.errorResponse(w, http.StatusInternalServerError, err)
			return
		}
	} else {
		if err := s.manager.UpdateAgent(&agent); err != nil {
			// If not found, maybe try create?
			// For strictness, let's try Update first.
			s.errorResponse(w, http.StatusInternalServerError, err)
			return
		}
	}

	s.jsonResponse(w, http.StatusOK, agent)
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.manager.DeleteAgent(id); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Sessions ---

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.manager.ListSessions()
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	s.jsonResponse(w, http.StatusOK, sessions)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, http.StatusBadRequest, err)
		return
	}

	sess, err := s.manager.NewSession(req.AgentID, "")
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}
	defer sess.Close()

	// Return the session info (not the full object, just ID/Path/etc)
	// Or we can return just ID.
	s.jsonResponse(w, http.StatusCreated, map[string]string{"id": sess.ID()})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := s.manager.LoadSession(id)
	if err != nil {
		s.errorResponse(w, http.StatusNotFound, err)
		return
	}
	defer sess.Close()

	// Get Context (History)
	entries, err := sess.GetContext()
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, err)
		return
	}

	s.jsonResponse(w, http.StatusOK, map[string]any{
		"header":  sess.Header(),
		"entries": entries,
	})
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
