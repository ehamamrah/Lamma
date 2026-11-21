package realtime

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/twitchtv/twirp"
)

// roomAPI is a thin authenticated client for LiveKit's RoomService (Twirp).
// It replaces the full server SDK, which would pull in a WebRTC client stack
// we do not need. Each call is signed with a short-lived, least-privilege
// admin token.
type roomAPI struct {
	svc         livekit.RoomService
	key, secret string
}

func newRoomAPI(baseURL, key, secret string) *roomAPI {
	return &roomAPI{
		svc: livekit.NewRoomServiceProtobufClient(baseURL, newHTTPClient()),
		key: key, secret: secret,
	}
}

// newHTTPClient is shared and pooled; per-call deadlines come from contexts.
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			MaxIdleConns:        32,
			MaxIdleConnsPerHost: 32,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

// authorize attaches a bearer token carrying exactly the grant needed.
func (a *roomAPI) authorize(ctx context.Context, grant auth.VideoGrant) (context.Context, error) {
	token, err := auth.NewAccessToken(a.key, a.secret).
		SetVideoGrant(&grant).SetValidFor(time.Minute).ToJWT()
	if err != nil {
		return nil, err
	}
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+token)
	return twirp.WithHTTPRequestHeaders(ctx, h)
}

func (a *roomAPI) roomCreate(ctx context.Context) (context.Context, error) {
	return a.authorize(ctx, auth.VideoGrant{RoomCreate: true})
}

func (a *roomAPI) roomAdmin(ctx context.Context, room string) (context.Context, error) {
	return a.authorize(ctx, auth.VideoGrant{RoomAdmin: true, Room: room})
}
