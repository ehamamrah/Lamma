package moderation

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
	"github.com/ehamamrah/lamma/backend/internal/participant"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
	"github.com/ehamamrah/lamma/backend/internal/platform/httpx"
)

// Handler exposes host-only endpoints on the {code} subrouter.
type Handler struct {
	svc   *Service
	limit func(http.Handler) http.Handler
}

// NewHandler creates the moderation handler; limit rate-limits per session.
func NewHandler(svc *Service, limit func(http.Handler) http.Handler) *Handler {
	return &Handler{svc: svc, limit: limit}
}

// Routes registers the moderation endpoints. Every route requires a host
// session (middleware) and the service re-checks the policy.
func (h *Handler) Routes(r chi.Router) {
	r.With(access.Require(access.ActionModerate)).Get("/participants", h.listWaiting)
	r.Group(func(r chi.Router) {
		r.Use(access.Require(access.ActionModerate), h.limit)
		r.Post("/participants/deny-waiting", h.denyWaiting)
		r.Post("/participants/{id}/admit", h.target(h.svc.Admit))
		r.Post("/participants/{id}/deny", h.target(h.svc.Deny))
		r.Post("/participants/{id}/remove", h.remove)
		r.Post("/participants/{id}/mute", h.mute)
		r.Patch("/settings", h.settings)
	})
	r.With(access.Require(access.ActionEnd), h.limit).Post("/end", h.end)
}

var errBadID = apperr.New(apperr.KindInvalid, "invalid_participant_id", "Invalid participant id.")

func targetID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, errBadID
	}
	return id, nil
}

type targetAction func(ctx context.Context, pr access.Principal, target uuid.UUID) error

func (h *Handler) target(action targetAction) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := targetID(r)
		if err == nil {
			pr, _ := access.FromContext(r.Context())
			err = action(r.Context(), pr, id)
		}
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) listWaiting(w http.ResponseWriter, r *http.Request) {
	pr, _ := access.FromContext(r.Context())
	ps, err := h.svc.ListWaiting(r.Context(), pr)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]participant.DTO, 0, len(ps))
	for _, p := range ps {
		out = append(out, participant.ToDTO(p))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"participants": out})
}

func (h *Handler) denyWaiting(w http.ResponseWriter, r *http.Request) {
	pr, _ := access.FromContext(r.Context())
	n, err := h.svc.DenyAllWaiting(r.Context(), pr)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]int{"denied": n})
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	id, err := targetID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	pr, _ := access.FromContext(r.Context())
	res, err := h.svc.Remove(r.Context(), pr, id)
	writeResult(w, r, res, err)
}

type muteRequest struct {
	Source string `json:"source"`
}

// validSources are the track sources a host may mute.
var validSources = map[string]bool{"microphone": true, "camera": true, "screen_share": true}

var errBadSource = apperr.New(apperr.KindInvalid, "invalid_source", "Source must be microphone, camera or screen_share.")

func (h *Handler) mute(w http.ResponseWriter, r *http.Request) {
	var req muteRequest
	id, err := targetID(r)
	if err == nil {
		err = httpx.Decode(w, r, &req)
	}
	if err == nil && !validSources[req.Source] {
		err = errBadSource
	}
	if err == nil {
		pr, _ := access.FromContext(r.Context())
		err = h.svc.Mute(r.Context(), pr, id, req.Source)
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type settingsRequest struct {
	Locked      *bool `json:"locked"`
	WaitingRoom *bool `json:"waitingRoom"`
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	pr, _ := access.FromContext(r.Context())
	m, err := h.svc.UpdateSettings(r.Context(), pr, meeting.Settings{Locked: req.Locked, WaitingRoom: req.WaitingRoom})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"locked": m.Locked, "waitingRoom": m.WaitingRoom})
}

func (h *Handler) end(w http.ResponseWriter, r *http.Request) {
	pr, _ := access.FromContext(r.Context())
	res, err := h.svc.End(r.Context(), pr)
	writeResult(w, r, res, err)
}

func writeResult(w http.ResponseWriter, r *http.Request, res Result, err error) {
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if res.LiveKitPending {
		httpx.JSON(w, http.StatusAccepted, map[string]bool{"livekitPending": true})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
