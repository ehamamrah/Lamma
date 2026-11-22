// Package realtime is the adapter to LiveKit: token signing, the room server
// API behind circuit breakers, and webhook ingestion. LiveKit carries all
// media; this package never touches media.
package realtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/livekit/protocol/livekit"
	"github.com/twitchtv/twirp"

	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
	"github.com/ehamamrah/lamma/backend/internal/platform/config"
	"github.com/ehamamrah/lamma/backend/internal/platform/metrics"
	"github.com/ehamamrah/lamma/backend/internal/platform/resilience"
)

// ErrNotFound means the room or participant does not exist in LiveKit.
var ErrNotFound = errors.New("livekit: not found")

// Client calls the LiveKit room service through three breakers so neither
// token traffic nor chat can disable moderation (PLAN.md §10).
type Client struct {
	api     *roomAPI
	room    *resilience.Breaker // CreateRoom (token path)
	admin   *resilience.Breaker // moderation, deletion, sweeps
	data    *resilience.Breaker // SendData (chat, notifications)
	metrics *metrics.Metrics
}

// NewClient creates a LiveKit client.
func NewClient(lk config.LiveKit, br config.Breaker, m *metrics.Metrics) *Client {
	mk := func(name string) *resilience.Breaker {
		return resilience.NewBreaker(resilience.Settings{
			Name: name, FailureThreshold: br.FailureThreshold, OpenTimeout: br.OpenTimeout,
			CallTimeout: lk.CallTimeout, IsSuccessful: isSuccessful,
			OnState: func(n string, v float64) { m.BreakerState.WithLabelValues(n).Set(v) },
		})
	}
	return &Client{
		api:  newRoomAPI(lk.URL, lk.APIKey, lk.APISecret),
		room: mk("livekit_room"), admin: mk("livekit_admin"), data: mk("livekit_data"),
		metrics: m,
	}
}

// EnsureRoom creates the room if needed (CreateRoom is idempotent).
func (c *Client) EnsureRoom(ctx context.Context, room string, maxParticipants int) error {
	return c.retry(ctx, c.room, "create_room", func(ctx context.Context) error {
		ctx, err := c.api.roomCreate(ctx)
		if err != nil {
			return err
		}
		_, err = c.api.svc.CreateRoom(ctx, &livekit.CreateRoomRequest{
			Name: room, EmptyTimeout: 300, DepartureTimeout: 20,
			MaxParticipants: uint32(maxParticipants), //nolint:gosec // validated 2..501
		})
		return err
	})
}

// DeleteRoom disconnects everyone and deletes the room; missing rooms are fine.
func (c *Client) DeleteRoom(ctx context.Context, room string) error {
	err := c.retry(ctx, c.admin, "delete_room", func(ctx context.Context) error {
		ctx, err := c.api.roomCreate(ctx)
		if err != nil {
			return err
		}
		_, err = c.api.svc.DeleteRoom(ctx, &livekit.DeleteRoomRequest{Room: room})
		return err
	})
	return ignoreNotFound(err)
}

// RemoveParticipant disconnects identity from room; already-gone is fine.
func (c *Client) RemoveParticipant(ctx context.Context, room, identity string) error {
	err := c.retry(ctx, c.admin, "remove_participant", func(ctx context.Context) error {
		ctx, err := c.api.roomAdmin(ctx, room)
		if err != nil {
			return err
		}
		_, err = c.api.svc.RemoveParticipant(ctx, &livekit.RoomParticipantIdentity{Room: room, Identity: identity})
		return err
	})
	return ignoreNotFound(err)
}

// ListIdentities returns the identities connected to room; exists is false
// when the room no longer exists in LiveKit.
func (c *Client) ListIdentities(ctx context.Context, room string) (ids []string, exists bool, err error) {
	err = c.retry(ctx, c.admin, "list_participants", func(ctx context.Context) error {
		var err error
		ids, err = c.listOnce(ctx, room)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return nil, false, nil
	}
	return ids, err == nil, err
}

func (c *Client) listOnce(ctx context.Context, room string) ([]string, error) {
	ctx, err := c.api.roomAdmin(ctx, room)
	if err != nil {
		return nil, err
	}
	res, err := c.api.svc.ListParticipants(ctx, &livekit.ListParticipantsRequest{Room: room})
	if err != nil {
		return nil, err
	}
	return identities(res.GetParticipants()), nil
}

func identities(ps []*livekit.ParticipantInfo) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.GetIdentity())
	}
	return out
}

// retry runs an idempotent op with retries behind br and maps failures.
func (c *Client) retry(ctx context.Context, br *resilience.Breaker, op string, fn func(context.Context) error) error {
	return c.observe(op, br.Retry(ctx, isRetryable, wrapTwirp(fn)))
}

// once runs a non-idempotent op exactly once behind br.
func (c *Client) once(ctx context.Context, br *resilience.Breaker, op string, fn func(context.Context) error) error {
	return c.observe(op, br.Do(ctx, wrapTwirp(fn)))
}

func (c *Client) observe(op string, err error) error {
	switch {
	case err == nil, errors.Is(err, ErrNotFound):
		return err
	case errors.Is(err, resilience.ErrOpen):
		return apperr.ErrLiveKitDown.WithRetry(10 * time.Second)
	}
	c.metrics.LiveKitFailures.WithLabelValues(op).Inc()
	return fmt.Errorf("livekit %s: %w", op, err)
}

// wrapTwirp converts twirp not-found errors into ErrNotFound.
func wrapTwirp(fn func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		err := fn(ctx)
		var te twirp.Error
		if errors.As(err, &te) && te.Code() == twirp.NotFound {
			return ErrNotFound
		}
		return err
	}
}

// clientCodes are caller errors: they must not trip breakers or be retried.
var clientCodes = map[twirp.ErrorCode]bool{
	twirp.NotFound: true, twirp.InvalidArgument: true, twirp.AlreadyExists: true,
	twirp.FailedPrecondition: true, twirp.OutOfRange: true, twirp.PermissionDenied: true,
}

func isClientError(err error) bool {
	if errors.Is(err, ErrNotFound) {
		return true
	}
	var te twirp.Error
	return errors.As(err, &te) && clientCodes[te.Code()]
}

func isSuccessful(err error) bool {
	return err == nil || errors.Is(err, context.Canceled) || isClientError(err)
}

func isRetryable(err error) bool {
	return !isClientError(err) && !errors.Is(err, context.Canceled)
}

func ignoreNotFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
