package participant

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
	"github.com/ehamamrah/lamma/backend/internal/platform/clientip"
	"github.com/ehamamrah/lamma/backend/internal/platform/httpx"
)

// Middleware is an HTTP middleware.
type Middleware = func(http.Handler) http.Handler

// HandlerConfig carries HTTP-level dependencies.
type HandlerConfig struct {
	Cookies       access.Cookies
	OriginHashKey []byte
	LiveKitURL    string
	JoinIPLimit   Middleware // per client IP
	JoinMtgLimit  Middleware // per meeting, sessionless callers only
	TokenLimit    Middleware // per session
}

// Handler exposes join, me, token and leave under /meetings/{code}.
type Handler struct {
	svc *Service
	cfg HandlerConfig
}

// NewHandler creates the participant HTTP handler.
func NewHandler(svc *Service, cfg HandlerConfig) *Handler { return &Handler{svc: svc, cfg: cfg} }

// Routes registers the participant endpoints on the {code} subrouter.
func (h *Handler) Routes(r chi.Router) {
	r.With(h.cfg.JoinIPLimit, h.cfg.JoinMtgLimit).Post("/join", h.join)
	r.Get("/me", h.me)
	r.With(access.Require(access.ActionJoinMedia), h.cfg.TokenLimit).Post("/token", h.token)
	r.Post("/leave", h.leave)
}

// DTO is the participant as seen by its owner or the host.
type DTO struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Status      string `json:"status"`
}

// ToDTO converts a participant to its API representation.
func ToDTO(p Participant) DTO {
	return DTO{ID: p.ID.String(), DisplayName: p.DisplayName, Role: string(p.Role), Status: string(p.Status)}
}

type joinRequest struct {
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

func (h *Handler) join(w http.ResponseWriter, r *http.Request) {
	var req joinRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.Join(r.Context(), h.joinInput(r, req))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if res.Token != "" {
		h.cfg.Cookies.Set(w, res.Meeting.Code, res.Token, res.Meeting.ExpiresAt)
	}
	status := http.StatusOK
	if res.Participant.Status == access.StatusWaiting {
		status = http.StatusAccepted
	}
	httpx.JSON(w, status, map[string]any{"participant": ToDTO(res.Participant)})
}

func (h *Handler) joinInput(r *http.Request, req joinRequest) JoinInput {
	addr := clientip.FromContext(r.Context())
	in := JoinInput{
		Code: chi.URLParam(r, "code"), DisplayName: req.DisplayName, Password: req.Password,
		Origin: clientip.Hash(h.cfg.OriginHashKey, addr), IPKey: clientip.Key(addr),
	}
	if p, ok := access.FromContext(r.Context()); ok {
		in.Session = &p
	}
	return in
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	pr, ok := access.FromContext(r.Context())
	if !ok {
		httpx.Error(w, r, apperr.ErrNoSession)
		return
	}
	p, err := h.svc.Current(r.Context(), pr)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"participant": ToDTO(p), "meetingOpen": pr.MeetingOpen(time.Now()),
	})
}

func (h *Handler) token(w http.ResponseWriter, r *http.Request) {
	pr, _ := access.FromContext(r.Context())
	grant, err := h.svc.IssueToken(r.Context(), pr)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"token": grant.Token, "url": h.cfg.LiveKitURL, "expiresAt": grant.ExpiresAt,
	})
}

func (h *Handler) leave(w http.ResponseWriter, r *http.Request) {
	pr, ok := access.FromContext(r.Context())
	if !ok {
		httpx.Error(w, r, apperr.ErrNoSession)
		return
	}
	if err := h.svc.Leave(r.Context(), pr); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
