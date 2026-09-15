package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ductringuyen-0618/feature-flag-api/internal/flag"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store/cached"
)

var identifier = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`)

type Server struct {
	svc *cached.Service
}

func New(svc *cached.Service) http.Handler {
	s := &Server{svc: svc}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	r.Get("/healthz", s.healthz)
	r.Get("/readyz", s.readyz)

	r.Route("/v1", func(r chi.Router) {
		r.Post("/flags", s.createFlag)
		r.Get("/flags", s.listFlags)
		r.Get("/flags/{name}", s.getFlag)
		r.Patch("/flags/{name}", s.patchFlag)
		r.Delete("/flags/{name}", s.deleteFlag)

		r.Put("/flags/{name}/users/{userID}", s.putOverride)
		r.Delete("/flags/{name}/users/{userID}", s.deleteOverride)

		r.Get("/evaluate/{name}", s.evaluateOne)
		r.Post("/evaluate", s.evaluateBulk)
	})
	return r
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	status := s.svc.Health(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Ready(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

type createFlagReq struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Enabled        bool   `json:"enabled"`
	RolloutPercent *int   `json:"rollout_percent"`
}

func (s *Server) createFlag(w http.ResponseWriter, r *http.Request) {
	var req createFlagReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if !requireID(w, req.Name, "name") {
		return
	}
	pct := 100
	if req.RolloutPercent != nil {
		pct = *req.RolloutPercent
	}
	if pct < 0 || pct > 100 {
		writeErr(w, http.StatusBadRequest, "rollout_percent must be 0-100")
		return
	}
	out, err := s.svc.CreateFlag(r.Context(), flag.Flag{
		Name:           req.Name,
		Description:    req.Description,
		Enabled:        req.Enabled,
		RolloutPercent: pct,
	})
	if errors.Is(err, store.ErrAlreadyExists) {
		writeErr(w, http.StatusConflict, "flag already exists")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create failed")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) listFlags(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListFlags(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"flags": list})
}

func (s *Server) getFlag(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !requireID(w, name, "name") {
		return
	}
	f, err := s.svc.GetFlag(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "flag not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get failed")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

type patchFlagReq struct {
	Enabled        *bool   `json:"enabled"`
	Description    *string `json:"description"`
	RolloutPercent *int    `json:"rollout_percent"`
}

func (s *Server) patchFlag(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !requireID(w, name, "name") {
		return
	}
	var req patchFlagReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Enabled == nil && req.Description == nil && req.RolloutPercent == nil {
		writeErr(w, http.StatusBadRequest, "no fields to update")
		return
	}
	if req.RolloutPercent != nil && (*req.RolloutPercent < 0 || *req.RolloutPercent > 100) {
		writeErr(w, http.StatusBadRequest, "rollout_percent must be 0-100")
		return
	}
	out, err := s.svc.UpdateFlag(r.Context(), name, req.Enabled, req.Description, req.RolloutPercent)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "flag not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteFlag(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !requireID(w, name, "name") {
		return
	}
	if err := s.svc.DeleteFlag(r.Context(), name); errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "flag not found")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type overrideReq struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) putOverride(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	userID := chi.URLParam(r, "userID")
	if !requireID(w, name, "name") || !requireID(w, userID, "user_id") {
		return
	}
	var req overrideReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	err := s.svc.SetOverride(r.Context(), flag.Override{FlagName: name, UserID: userID, Enabled: req.Enabled})
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "flag not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "override failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"flag_name": name,
		"user_id":   userID,
		"enabled":   req.Enabled,
	})
}

func (s *Server) deleteOverride(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	userID := chi.URLParam(r, "userID")
	if !requireID(w, name, "name") || !requireID(w, userID, "user_id") {
		return
	}
	if err := s.svc.DeleteOverride(r.Context(), name, userID); errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "override not found")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete override failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) evaluateOne(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeErr(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if !requireID(w, name, "name") || !requireID(w, userID, "user_id") {
		return
	}
	res, err := s.svc.Evaluate(r.Context(), name, userID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "flag not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "evaluate failed")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type bulkEvalReq struct {
	UserID string   `json:"user_id"`
	Flags  []string `json:"flags"`
}

func (s *Server) evaluateBulk(w http.ResponseWriter, r *http.Request) {
	var req bulkEvalReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.UserID == "" {
		writeErr(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if !requireID(w, req.UserID, "user_id") {
		return
	}
	if len(req.Flags) == 0 {
		writeErr(w, http.StatusBadRequest, "flags is required")
		return
	}
	for _, name := range req.Flags {
		if !requireID(w, name, "name") {
			return
		}
	}
	res, err := s.svc.EvaluateBulk(r.Context(), req.UserID, req.Flags)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "evaluate failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": req.UserID, "results": res})
}

func requireID(w http.ResponseWriter, value, field string) bool {
	if identifier.MatchString(value) {
		return true
	}
	writeErr(w, http.StatusBadRequest, "invalid "+field)
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
