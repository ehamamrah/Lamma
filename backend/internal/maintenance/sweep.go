package maintenance

import (
	"context"

	"github.com/google/uuid"
)

// sweep reconciles live rooms with the database so lost webhooks cannot
// leave intruders connected or seats stuck (PLAN.md §10 step 5).
func (w *Worker) sweep(ctx context.Context) error {
	ids, err := w.meetings.ClaimRoomsToSweep(ctx)
	if err != nil {
		return err
	}
	return fanOut(ctx, ids, w.sweepRoom)
}

func (w *Worker) sweepRoom(ctx context.Context, meetingID uuid.UUID) error {
	connected, err := w.lk.ListIdentities(ctx, meetingID.String())
	if isRoomGone(err) {
		if err := w.parts.MarkRoomEmpty(ctx, meetingID); err != nil {
			return err
		}
		return w.meetings.ClearRoomCreated(ctx, meetingID)
	}
	if err != nil {
		return err
	}
	if err := w.evictUnadmitted(ctx, meetingID, connected); err != nil {
		return err
	}
	return w.clearGhosts(ctx, meetingID, connected)
}

// evictUnadmitted removes connected identities that are not admitted.
func (w *Worker) evictUnadmitted(ctx context.Context, meetingID uuid.UUID, connected []string) error {
	admitted, err := w.parts.AdmittedIDs(ctx, meetingID)
	if err != nil {
		return err
	}
	allowed := toSet(admitted)
	for _, identity := range connected {
		if _, ok := allowed[identity]; ok {
			continue
		}
		if err := w.lk.RemoveParticipant(ctx, meetingID.String(), identity); err != nil {
			return err
		}
	}
	return nil
}

// clearGhosts frees seats of participants the DB thinks are connected but
// LiveKit does not list (a lost participant_left webhook).
func (w *Worker) clearGhosts(ctx context.Context, meetingID uuid.UUID, connected []string) error {
	inRoom, err := w.parts.InRoomIDs(ctx, meetingID)
	if err != nil {
		return err
	}
	live := make(map[string]struct{}, len(connected))
	for _, id := range connected {
		live[id] = struct{}{}
	}
	for _, id := range inRoom {
		if _, ok := live[id.String()]; ok {
			continue
		}
		if err := w.parts.MarkNotInRoom(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func toSet(ids []uuid.UUID) map[string]struct{} {
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id.String()] = struct{}{}
	}
	return out
}
