package realtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/twitchtv/twirp"

	"github.com/ehamamrah/lamma/backend/internal/participant"
)

func TestSignerGrantsAreMinimal(t *testing.T) {
	s := NewSigner("key", "secret-secret-secret-secret-secret", 2*time.Minute)
	token, exp, err := s.Sign(participant.Grant{Room: "room-1", Identity: "id-1", Name: "Ann", Role: "guest"})
	if err != nil {
		t.Fatal(err)
	}
	if until := time.Until(exp); until > 2*time.Minute || until < time.Minute {
		t.Fatalf("expiry %v not ~2m", until)
	}
	v, err := auth.ParseAPIToken(token)
	if err != nil {
		t.Fatal(err)
	}
	_, claims, err := v.Verify("secret-secret-secret-secret-secret")
	if err != nil {
		t.Fatal(err)
	}
	g := claims.Video
	switch {
	case !g.RoomJoin || g.Room != "room-1":
		t.Fatal("token must join exactly its room")
	case g.RoomAdmin || g.RoomCreate || g.RoomList || g.RoomRecord:
		t.Fatal("participants must never get admin grants")
	case !g.GetCanPublishData():
		t.Fatal("data publish is needed for reactions and the whiteboard")
	case g.GetCanUpdateOwnMetadata():
		t.Fatal("participants must not change their own metadata (role display)")
	case v.Identity() != "id-1":
		t.Fatal("identity must be the server-generated participant id")
	}
}

func TestErrorClassification(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		successful bool
		retryable  bool
	}{
		{"nil", nil, true, false},
		{"not found", twirp.NewError(twirp.NotFound, "x"), true, false},
		{"invalid argument", twirp.NewError(twirp.InvalidArgument, "x"), true, false},
		{"caller cancelled", context.Canceled, true, false},
		{"unavailable", twirp.NewError(twirp.Unavailable, "x"), false, true},
		{"internal", twirp.NewError(twirp.Internal, "x"), false, true},
		{"network", errors.New("connection refused"), false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSuccessful(tt.err); got != tt.successful {
				t.Fatalf("isSuccessful = %v", got)
			}
			if tt.err != nil && isRetryable(tt.err) != tt.retryable {
				t.Fatalf("isRetryable = %v", !tt.retryable)
			}
		})
	}
}

func TestWrapTwirpMapsNotFound(t *testing.T) {
	err := wrapTwirp(func(context.Context) error { return twirp.NewError(twirp.NotFound, "room") })(context.Background())
	if !errors.Is(err, ErrNotFound) || ignoreNotFound(err) != nil {
		t.Fatalf("got %v", err)
	}
}
