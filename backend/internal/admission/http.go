package admission

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/platform/clientip"
	"github.com/ehamamrah/lamma/backend/internal/platform/httpx"
)

const (
	heartbeat     = 25 * time.Second
	maxLifetime   = 10 * time.Minute
	writeDeadline = 10 * time.Second
)

// StatusReader returns the caller's current status from the database.
type StatusReader interface {
	CurrentStatus(ctx context.Context, pr access.Principal) (access.Status, error)
}

// Handler serves GET /admission/events.
type Handler struct {
	hub    *Hub
	status StatusReader
}

// NewHandler creates the SSE handler.
func NewHandler(hub *Hub, status StatusReader) *Handler { return &Handler{hub: hub, status: status} }

// Routes registers the SSE endpoint on the {code} subrouter.
func (h *Handler) Routes(r chi.Router) {
	r.With(access.Require(access.ActionAwait)).Get("/admission/events", h.events)
}

// events registers with the hub FIRST and then reads the current status, so a
// decision made in between can never be missed (PLAN.md §9).
func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	pr, _ := access.FromContext(r.Context())
	decisions, release, err := h.hub.Register(pr.ParticipantID, clientip.Key(clientip.FromContext(r.Context())))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	defer release()
	current, err := h.status.CurrentStatus(r.Context(), pr)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	s := newStream(w)
	if current != access.StatusWaiting {
		s.send("decision", string(current))
		return
	}
	h.wait(r.Context(), s, decisions)
}

func (h *Handler) wait(ctx context.Context, s *stream, decisions <-chan access.Status) {
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	deadline := time.NewTimer(maxLifetime)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			s.send("timeout", "reconnect")
			return
		case <-h.hub.Done():
			s.send("shutdown", "reconnect")
			return
		case d := <-decisions:
			s.send("decision", string(d))
			return
		case <-ticker.C:
			if !s.comment("ping") {
				return
			}
		}
	}
}

// stream writes Server-Sent Events with a per-write deadline (the server's
// global WriteTimeout would otherwise cut long-lived streams).
type stream struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func newStream(w http.ResponseWriter) *stream {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	s := &stream{w: w, rc: http.NewResponseController(w)}
	s.write("retry: 5000\n\n") // client reconnect delay; also flushes headers now
	return s
}

func (s *stream) send(event, data string) bool {
	return s.write(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
}

func (s *stream) comment(text string) bool { return s.write(": " + text + "\n\n") }

func (s *stream) write(payload string) bool {
	_ = s.rc.SetWriteDeadline(time.Now().Add(writeDeadline))
	if _, err := fmt.Fprint(s.w, payload); err != nil {
		return false
	}
	return s.rc.Flush() == nil
}
