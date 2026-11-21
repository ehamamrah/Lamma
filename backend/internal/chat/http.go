package chat

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/platform/httpx"
)

// Handler serves GET/POST /chat on the {code} subrouter.
type Handler struct {
	svc          *Service
	sessionLimit func(http.Handler) http.Handler
}

// NewHandler creates the chat handler; sessionLimit rate-limits per session.
func NewHandler(svc *Service, sessionLimit func(http.Handler) http.Handler) *Handler {
	return &Handler{svc: svc, sessionLimit: sessionLimit}
}

// Routes registers the chat endpoints.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(access.Require(access.ActionChat))
		r.Get("/chat", h.list)
		r.With(h.sessionLimit).Post("/chat", h.send)
	})
}

type sendRequest struct {
	Text string `json:"text"`
}

func (h *Handler) send(w http.ResponseWriter, r *http.Request) {
	var req sendRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	pr, _ := access.FromContext(r.Context())
	msg, err := h.svc.Send(r.Context(), pr, req.Text)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, msg)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	pr, _ := access.FromContext(r.Context())
	httpx.JSON(w, http.StatusOK, map[string]any{"messages": h.svc.Recent(r.Context(), pr)})
}
