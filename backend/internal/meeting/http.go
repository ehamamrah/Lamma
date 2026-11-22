package meeting

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/platform/clientip"
	"github.com/ehamamrah/lamma/backend/internal/platform/httpx"
)

// Handler exposes the public meeting endpoints.
type Handler struct {
	svc       *Service
	cookies   access.Cookies
	baseURL   string
	originKey []byte
	limitIP   func(http.Handler) http.Handler // create (fail-closed)
	lookupIP  func(http.Handler) http.Handler
}

// HandlerConfig carries the HTTP-level dependencies.
type HandlerConfig struct {
	Cookies       access.Cookies
	PublicBaseURL string
	OriginHashKey []byte
	CreateLimit   func(http.Handler) http.Handler
	LookupLimit   func(http.Handler) http.Handler
}

// NewHandler creates the meeting HTTP handler.
func NewHandler(svc *Service, c HandlerConfig) *Handler {
	return &Handler{svc: svc, cookies: c.Cookies, baseURL: c.PublicBaseURL, originKey: c.OriginHashKey,
		limitIP: c.CreateLimit, lookupIP: c.LookupLimit}
}

// RootRoutes registers POST /meetings on the /api/v1 router.
func (h *Handler) RootRoutes(r chi.Router) {
	r.With(h.limitIP).Post("/meetings", h.create)
}

// Routes registers GET / on the /meetings/{code} subrouter.
func (h *Handler) Routes(r chi.Router) {
	r.With(h.lookupIP).Get("/", h.lookup)
}

type createRequest struct {
	DisplayName      string `json:"displayName"`
	Title            string `json:"title"`
	Password         string `json:"password"`
	WaitingRoom      bool   `json:"waitingRoom"`
	ExpiresInMinutes int    `json:"expiresInMinutes"`
}

// DTO is the public view of a meeting.
type DTO struct {
	Code             string    `json:"code"`
	URL              string    `json:"url,omitempty"`
	Title            string    `json:"title"`
	RequiresPassword bool      `json:"requiresPassword"`
	WaitingRoom      bool      `json:"waitingRoom"`
	Locked           bool      `json:"locked"`
	ExpiresAt        time.Time `json:"expiresAt"`
}

type participantDTO struct {
	ID     string `json:"id"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	out, err := h.svc.Create(r.Context(), h.toInput(r, req))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	h.cookies.Set(w, out.Meeting.Code, out.Host.Token, out.Meeting.ExpiresAt)
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"meeting":     h.toDTO(out.Meeting, true),
		"participant": participantDTO{ID: out.Host.ParticipantID.String(), Role: "host", Status: "admitted"},
	})
}

func (h *Handler) toInput(r *http.Request, req createRequest) CreateInput {
	return CreateInput{
		DisplayName: req.DisplayName, Title: req.Title, Password: req.Password,
		WaitingRoom: req.WaitingRoom, ExpiresIn: minutes(req.ExpiresInMinutes),
		Origin: clientip.Hash(h.originKey, clientip.FromContext(r.Context())),
	}
}

// minutes converts the request value; 0 means "default". Out-of-range input
// maps to a negative duration so the service rejects it with invalid_expiry.
func minutes(n int) time.Duration {
	if n < 0 || n > 60*24*366 {
		return -1
	}
	return time.Duration(n) * time.Minute
}

func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	m, err := h.svc.Lookup(r.Context(), chi.URLParam(r, "code"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, h.toDTO(m, false))
}

func (h *Handler) toDTO(m Meeting, withURL bool) DTO {
	dto := DTO{
		Code: m.Code, Title: m.Title, RequiresPassword: m.RequiresPassword(),
		WaitingRoom: m.WaitingRoom, Locked: m.Locked, ExpiresAt: m.ExpiresAt,
	}
	if withURL {
		dto.URL = h.baseURL + "/m/" + m.Code
	}
	return dto
}
