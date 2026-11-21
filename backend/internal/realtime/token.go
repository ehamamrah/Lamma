package realtime

import (
	"encoding/json"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"

	"github.com/ehamamrah/lamma/backend/internal/participant"
)

// Signer issues short-lived LiveKit access tokens. Signing is local: no
// network call, so token issuance never depends on LiveKit availability
// beyond room creation.
type Signer struct {
	key, secret string
	ttl         time.Duration
	now         func() time.Time
}

// NewSigner creates a Signer.
func NewSigner(key, secret string, ttl time.Duration) *Signer {
	return &Signer{key: key, secret: secret, ttl: ttl, now: time.Now}
}

// Sign returns a JWT with the minimal grant set (PLAN.md §5). Host powers
// live in the API, so host and guest grants are identical. Nil grant fields
// default to "allowed" in LiveKit, so every permission is set explicitly.
func (s *Signer) Sign(g participant.Grant) (string, time.Time, error) {
	grant := &auth.VideoGrant{RoomJoin: true, Room: g.Room}
	grant.SetCanPublish(true)
	grant.SetCanSubscribe(true)
	grant.SetCanPublishData(false)
	grant.SetCanUpdateOwnMetadata(false)
	grant.SetCanPublishSources([]livekit.TrackSource{
		livekit.TrackSource_CAMERA, livekit.TrackSource_MICROPHONE,
		livekit.TrackSource_SCREEN_SHARE, livekit.TrackSource_SCREEN_SHARE_AUDIO,
	})
	meta, _ := json.Marshal(map[string]string{"role": g.Role}) // display only
	token, err := auth.NewAccessToken(s.key, s.secret).
		SetVideoGrant(grant).SetIdentity(g.Identity).SetName(g.Name).
		SetMetadata(string(meta)).SetValidFor(s.ttl).ToJWT()
	return token, s.now().Add(s.ttl), err
}
