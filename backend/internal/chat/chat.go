// Package chat implements meeting chat. Messages go through the API (so they
// are validated and rate-limited) and are fanned out by LiveKit SendData;
// clients have no data-publish permission (PLAN.md A4, ADR-004).
package chat

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
	"github.com/ehamamrah/lamma/backend/internal/platform/text"
)

// Topic is the LiveKit data topic clients listen on.
const Topic = "lamma.chat"

const maxLen = 1000

// Message is a chat message as delivered to clients.
type Message struct {
	ID            string    `json:"id"`
	ParticipantID string    `json:"participantId"`
	DisplayName   string    `json:"displayName"`
	Text          string    `json:"text"`
	SentAt        time.Time `json:"sentAt"`
}

// Broadcaster sends a data message to a LiveKit room.
type Broadcaster interface {
	SendData(ctx context.Context, room, topic string, payload []byte, to ...string) error
}

// Buffer keeps recent messages for late joiners (Redis; not a source of truth).
type Buffer interface {
	Append(ctx context.Context, meetingID uuid.UUID, payload []byte, until time.Time) error
	Recent(ctx context.Context, meetingID uuid.UUID) ([][]byte, error)
	Clear(ctx context.Context, meetingID uuid.UUID) error
}

// RateCheck consumes one unit of a rate-limit policy for key.
type RateCheck func(ctx context.Context, key string) error

// Service sends and lists chat messages.
type Service struct {
	bc           Broadcaster
	buffer       Buffer
	meetingLimit RateCheck
	now          func() time.Time
}

// NewService creates the chat service.
func NewService(bc Broadcaster, buffer Buffer, meetingLimit RateCheck) *Service {
	return &Service{bc: bc, buffer: buffer, meetingLimit: meetingLimit, now: time.Now}
}

var errInvalidText = apperr.New(apperr.KindInvalid, "invalid_message", "Messages must be 1–1000 characters.")

// Send validates, rate-limits per meeting, broadcasts and buffers a message.
func (s *Service) Send(ctx context.Context, pr access.Principal, raw string) (Message, error) {
	body, ok := text.CleanMultiline(raw, maxLen)
	if !ok {
		return Message{}, errInvalidText
	}
	if err := s.meetingLimit(ctx, pr.MeetingID.String()); err != nil {
		return Message{}, err
	}
	msg, payload, err := s.build(pr, body)
	if err != nil {
		return Message{}, err
	}
	if err := s.bc.SendData(ctx, pr.MeetingID.String(), Topic, payload); err != nil {
		return Message{}, err
	}
	if err := s.buffer.Append(ctx, pr.MeetingID, payload, pr.ExpiresAt); err != nil {
		slog.WarnContext(ctx, "chat buffer append failed", slog.Any("error", err))
	}
	return msg, nil
}

func (s *Service) build(pr access.Principal, body string) (Message, []byte, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Message{}, nil, err
	}
	msg := Message{
		ID: id.String(), ParticipantID: pr.ParticipantID.String(),
		DisplayName: pr.DisplayName, Text: body, SentAt: s.now().UTC(),
	}
	payload, err := json.Marshal(msg)
	return msg, payload, err
}

// Recent returns buffered messages, oldest first. Buffer failures degrade to
// an empty history rather than an error.
func (s *Service) Recent(ctx context.Context, pr access.Principal) []json.RawMessage {
	raw, err := s.buffer.Recent(ctx, pr.MeetingID)
	if err != nil {
		slog.WarnContext(ctx, "chat buffer read failed", slog.Any("error", err))
	}
	out := make([]json.RawMessage, 0, len(raw))
	for i := len(raw) - 1; i >= 0; i-- {
		out = append(out, raw[i])
	}
	return out
}

// Clear drops the history when a meeting ends.
func (s *Service) Clear(ctx context.Context, meetingID uuid.UUID) {
	if err := s.buffer.Clear(ctx, meetingID); err != nil {
		slog.WarnContext(ctx, "chat buffer clear failed", slog.Any("error", err))
	}
}
