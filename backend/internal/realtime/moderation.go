package realtime

import (
	"context"

	"github.com/livekit/protocol/livekit"
)

// Source names accepted by MuteSource.
var sources = map[string]livekit.TrackSource{
	"microphone":   livekit.TrackSource_MICROPHONE,
	"camera":       livekit.TrackSource_CAMERA,
	"screen_share": livekit.TrackSource_SCREEN_SHARE,
}

// ValidSource reports whether name is a mutable track source.
func ValidSource(name string) bool { _, ok := sources[name]; return ok }

// MuteSource server-mutes every published track of the given source. Lookup
// and mute are retried together as one idempotent unit. Hosts can never
// unmute others (privacy, PLAN.md A6).
func (c *Client) MuteSource(ctx context.Context, room, identity, source string) error {
	want := sources[source]
	err := c.retry(ctx, c.admin, "mute", func(ctx context.Context) error {
		ctx, err := c.api.roomAdmin(ctx, room)
		if err != nil {
			return err
		}
		p, err := c.api.svc.GetParticipant(ctx, &livekit.RoomParticipantIdentity{Room: room, Identity: identity})
		if err != nil {
			return err
		}
		return c.muteTracks(ctx, room, identity, p.GetTracks(), want)
	})
	return ignoreNotFound(err)
}

func (c *Client) muteTracks(ctx context.Context, room, identity string, tracks []*livekit.TrackInfo, src livekit.TrackSource) error {
	for _, t := range tracks {
		if t.GetSource() != src || t.GetMuted() {
			continue
		}
		_, err := c.api.svc.MutePublishedTrack(ctx, &livekit.MuteRoomTrackRequest{
			Room: room, Identity: identity, TrackSid: t.GetSid(), Muted: true,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// SendData delivers a reliable data message to the room (or to the given
// identities). It is not idempotent, so it is never retried.
func (c *Client) SendData(ctx context.Context, room, topic string, payload []byte, to ...string) error {
	return c.once(ctx, c.data, "send_data", func(ctx context.Context) error {
		ctx, err := c.api.roomAdmin(ctx, room)
		if err != nil {
			return err
		}
		_, err = c.api.svc.SendData(ctx, &livekit.SendDataRequest{
			Room: room, Data: payload, Kind: livekit.DataPacket_RELIABLE,
			Topic: &topic, DestinationIdentities: to,
		})
		return err
	})
}
