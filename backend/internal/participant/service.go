package participant

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
)

// Store is the persistence port for participants.
type Store interface {
	LockMeeting(ctx context.Context, meetingID uuid.UUID) (MeetingLock, error)
	Insert(ctx context.Context, n NewParticipant) (Participant, error)
	Get(ctx context.Context, meetingID, id uuid.UUID) (Participant, error)
	Transition(ctx context.Context, t Transition) (Participant, error)
	CountWaiting(ctx context.Context, meetingID uuid.UUID, origin []byte) (byOrigin, total int64, err error)
	ListWaiting(ctx context.Context, meetingID uuid.UUID) ([]Participant, error)
	DenyAllWaiting(ctx context.Context, meetingID, actor uuid.UUID) ([]uuid.UUID, error)
	CountSeats(ctx context.Context, meetingID, exclude uuid.UUID, origin []byte) (total, unconnected int64, err error)
	ClaimLease(ctx context.Context, id uuid.UUID, ttl time.Duration) (leaseCount int, inRoom bool, err error)
	ReleaseLease(ctx context.Context, id uuid.UUID) error
	AnyoneInRoom(ctx context.Context, meetingID uuid.UUID) (bool, error)
	Audit(ctx context.Context, meetingID, actor uuid.UUID, action string, target uuid.UUID) error
}

// MeetingLock is the meeting row locked FOR UPDATE inside a transaction.
type MeetingLock struct {
	MaxParticipants int
	Locked          bool
	WaitingRoom     bool
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

// Meetings is the subset of the meeting module this module reads.
type Meetings interface {
	Lookup(ctx context.Context, code string) (meeting.Meeting, error)
	GetOpenByID(ctx context.Context, id uuid.UUID) (meeting.Meeting, error)
	MarkRoomCreated(ctx context.Context, id uuid.UUID) error
}

// Rooms creates LiveKit rooms (idempotent).
type Rooms interface {
	EnsureRoom(ctx context.Context, room string, maxParticipants int) error
}

// Grant describes a LiveKit token to sign.
type Grant struct {
	Room     string
	Identity string
	Name     string
	Role     string
}

// TokenSigner signs LiveKit access tokens locally.
type TokenSigner interface {
	Sign(g Grant) (token string, expiresAt time.Time, err error)
}

// Notifier delivers waiting-room events (best effort, after commit).
type Notifier interface {
	Decided(ctx context.Context, participantID uuid.UUID, decision access.Status)
	Waiting(ctx context.Context, meetingID uuid.UUID)
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

// Deps groups the service dependencies.
type Deps struct {
	Store         Store
	Meetings      Meetings
	Rooms         Rooms
	Signer        TokenSigner
	Notifier      Notifier
	Passwords     PasswordVerifier
	Tx            Transactor
	PasswordLimit RateCheck
	LeaseTTL      time.Duration
}

// Service implements the participant use cases.
type Service struct {
	Deps
	now func() time.Time
}

// NewService wires the participant service.
func NewService(d Deps) *Service { return &Service{Deps: d, now: time.Now} }
