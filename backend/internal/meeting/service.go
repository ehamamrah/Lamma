package meeting

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/platform/text"
)

// Store is the persistence port for meetings.
type Store interface {
	Insert(ctx context.Context, m Meeting) error // ErrCodeTaken on collision
	GetOpenByCode(ctx context.Context, code string) (Meeting, error)
	GetOpenByID(ctx context.Context, id uuid.UUID) (Meeting, error)
	UpdateSettings(ctx context.Context, id, actor uuid.UUID, s Settings) (Meeting, error)
	End(ctx context.Context, id uuid.UUID, actor *uuid.UUID) (bool, error)
	RoomStore
}

// RoomStore records the LiveKit room lifecycle for a meeting.
type RoomStore interface {
	IsOpen(ctx context.Context, id uuid.UUID) (bool, error)
	MarkRoomCreated(ctx context.Context, id uuid.UUID) error
	ClearRoomCreated(ctx context.Context, id uuid.UUID) error
	MarkRoomClosed(ctx context.Context, id uuid.UUID) error
	ReopenRoomForClosing(ctx context.Context, id uuid.UUID) error
}

// Host is the creator's session, registered by the participant module.
type Host struct {
	ParticipantID uuid.UUID
	Token         string
}

// HostRegistrar creates the host participant inside the creation transaction.
type HostRegistrar interface {
	RegisterHost(ctx context.Context, meetingID uuid.UUID, displayName string, origin []byte) (Host, error)
}

// Transactor runs fn atomically.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// PasswordHasher hashes meeting passwords.
type PasswordHasher interface {
	Hash(ctx context.Context, pw string) (string, error)
}

// Config bounds meeting creation.
type Config struct {
	DefaultTTL time.Duration
	MaxTTL     time.Duration
}

// Service implements the meeting use cases.
type Service struct {
	store  Store
	hosts  HostRegistrar
	tx     Transactor
	hasher PasswordHasher
	cfg    Config
	now    func() time.Time
}

// NewService wires the meeting service.
func NewService(store Store, hosts HostRegistrar, tx Transactor, hasher PasswordHasher, cfg Config) *Service {
	return &Service{store: store, hosts: hosts, tx: tx, hasher: hasher, cfg: cfg, now: time.Now}
}

// CreateInput is a validated-on-use creation request.
type CreateInput struct {
	DisplayName string
	Title       string
	Password    string
	WaitingRoom bool
	ExpiresIn   time.Duration // 0 = default
	Origin      []byte
}

// Created is the result of Create.
type Created struct {
	Meeting Meeting
	Host    Host
}

const minTTL = 15 * time.Minute

// Create makes a meeting and its host atomically. No LiveKit call is made:
// rooms are created lazily on the first token request.
func (s *Service) Create(ctx context.Context, in CreateInput) (Created, error) {
	m, name, err := s.prepare(ctx, in)
	if err != nil {
		return Created{}, err
	}
	var host Host
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.insertWithFreshCode(ctx, &m); err != nil {
			return err
		}
		host, err = s.hosts.RegisterHost(ctx, m.ID, name, in.Origin)
		return err
	})
	if err != nil {
		return Created{}, fmt.Errorf("create meeting: %w", err)
	}
	return Created{Meeting: m, Host: host}, nil
}

func (s *Service) prepare(ctx context.Context, in CreateInput) (Meeting, string, error) {
	name, title, ttl, err := s.validate(in)
	if err != nil {
		return Meeting{}, "", err
	}
	hash, err := s.hashPassword(ctx, in.Password)
	if err != nil {
		return Meeting{}, "", err
	}
	now := s.now()
	return Meeting{
		ID: uuid.New(), Title: title, PasswordHash: hash, WaitingRoom: in.WaitingRoom,
		CreatedAt: now, ExpiresAt: now.Add(ttl),
	}, name, nil
}

func (s *Service) validate(in CreateInput) (name, title string, ttl time.Duration, err error) {
	name, ok := text.Clean(in.DisplayName, 1, 40)
	if !ok {
		return "", "", 0, errInvalidName
	}
	title, ok = text.Clean(in.Title, 0, 120)
	if !ok {
		return "", "", 0, errInvalidTitle
	}
	if n := utf8.RuneCountInString(in.Password); in.Password != "" && (n < 8 || n > 64) {
		return "", "", 0, errInvalidPassword
	}
	ttl = in.ExpiresIn
	if ttl == 0 {
		ttl = s.cfg.DefaultTTL
	}
	if ttl < minTTL || ttl > s.cfg.MaxTTL {
		return "", "", 0, errInvalidExpiry
	}
	return name, title, ttl, nil
}

func (s *Service) hashPassword(ctx context.Context, pw string) (string, error) {
	if pw == "" {
		return "", nil
	}
	return s.hasher.Hash(ctx, pw)
}

// insertWithFreshCode retries on the (astronomically rare) code collision.
func (s *Service) insertWithFreshCode(ctx context.Context, m *Meeting) error {
	for range 3 {
		code, err := NewCode()
		if err != nil {
			return err
		}
		m.Code = code
		err = s.store.Insert(ctx, *m)
		if !errors.Is(err, ErrCodeTaken) {
			return err
		}
	}
	return errors.New("could not allocate a unique meeting code")
}

// Lookup returns an open meeting by its canonical code.
func (s *Service) Lookup(ctx context.Context, code string) (Meeting, error) {
	if !ValidCode(code) {
		return Meeting{}, errNotFound()
	}
	return s.store.GetOpenByCode(ctx, code)
}
