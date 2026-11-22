package participant

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
)

// Store is the persistence port for membership (join, waiting room, moderation).
type Store interface {
	LockMeeting(ctx context.Context, meetingID uuid.UUID) (MeetingLock, error)
	Insert(ctx context.Context, n NewParticipant) (Participant, error)
	Get(ctx context.Context, meetingID, id uuid.UUID) (Participant, error)
	Transition(ctx context.Context, t Transition) (Participant, error)
	RemoveGuest(ctx context.Context, meetingID, actor, target uuid.UUID) (previous access.Status, err error)
	CountWaiting(ctx context.Context, meetingID uuid.UUID, origin []byte) (byOrigin, total int64, err error)
	ListWaiting(ctx context.Context, meetingID uuid.UUID) ([]Participant, error)
	DenyAllWaiting(ctx context.Context, meetingID, actor uuid.UUID) ([]uuid.UUID, error)
	HostID(ctx context.Context, meetingID uuid.UUID) (uuid.UUID, error)
	ReleaseLease(ctx context.Context, id uuid.UUID) error
	Audit(ctx context.Context, meetingID, actor uuid.UUID, action string, target uuid.UUID) error
}

// MeetingLock is the meeting row locked FOR UPDATE inside a transaction.
type MeetingLock struct {
	Locked      bool
	WaitingRoom bool
}

// NewParticipant is the data needed to insert a participant.
type NewParticipant struct {
	MeetingID   uuid.UUID
	DisplayName string
	Role        access.Role
	Status      access.Status
	Origin      []byte
	SessionHash []byte
}

// Transition changes a participant's status if it is currently one of From,
// and records an audit event when Action is set — atomically.
type Transition struct {
	MeetingID uuid.UUID
	ID        uuid.UUID
	To        access.Status
	From      []access.Status
	Actor     *uuid.UUID
	Action    string
}

// MeetingLookup resolves meetings by public code.
type MeetingLookup interface {
	Lookup(ctx context.Context, code string) (meeting.Meeting, error)
}

// Notifier delivers waiting-room events (best effort, after commit).
type Notifier interface {
	Decided(ctx context.Context, participantID uuid.UUID, decision access.Status)
	Waiting(ctx context.Context, meetingID, hostID uuid.UUID)
}

// PasswordVerifier checks meeting passwords (argon2 gate).
type PasswordVerifier interface {
	Verify(ctx context.Context, encoded, pw string) (bool, error)
}

// Transactor runs fn atomically.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// RateCheck consumes one unit of a rate-limit policy for key.
type RateCheck func(ctx context.Context, key string) error

// Deps groups the membership service dependencies.
type Deps struct {
	Store         Store
	Meetings      MeetingLookup
	Notifier      Notifier
	Passwords     PasswordVerifier
	Tx            Transactor
	PasswordLimit RateCheck
}

// Service implements joining, the waiting room and moderation state changes.
// Seat leases and tokens live in Seats.
type Service struct {
	d   Deps
	now func() time.Time
}

// NewService wires the participant service.
func NewService(d Deps) *Service { return &Service{d: d, now: time.Now} }
