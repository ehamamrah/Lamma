-- name: InsertMeeting :execrows
-- ON CONFLICT keeps the surrounding transaction usable on a code collision.
INSERT INTO meetings (id, code, title, password_hash, waiting_room, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (code) DO NOTHING;

-- name: GetOpenMeetingByCode :one
-- Liveness guard: ended or expired meetings are indistinguishable from unknown ones.
SELECT * FROM meetings
WHERE code = $1 AND status = 'active' AND expires_at > now();

-- name: GetOpenMeetingByID :one
SELECT * FROM meetings
WHERE id = $1 AND status = 'active' AND expires_at > now();

-- name: UpdateMeetingSettings :one
UPDATE meetings
SET locked       = coalesce(sqlc.narg(locked), locked),
    waiting_room = coalesce(sqlc.narg(waiting_room), waiting_room)
WHERE id = $1 AND status = 'active' AND expires_at > now()
RETURNING *;

-- name: EndMeeting :one
UPDATE meetings SET status = 'ended', ended_at = now()
WHERE id = $1 AND status = 'active'
RETURNING id;

-- name: InsertModerationEvent :exec
INSERT INTO moderation_events (meeting_id, actor_id, action, target_id)
VALUES ($1, $2, $3, $4);

-- name: MarkRoomCreated :exec
UPDATE meetings SET room_created_at = now(), sweep_after = now() + interval '5 minutes'
WHERE id = $1;

-- name: ClearRoomCreated :exec
UPDATE meetings SET room_created_at = NULL WHERE id = $1;

-- name: MarkRoomClosed :exec
UPDATE meetings SET room_closed_at = now(), room_created_at = NULL WHERE id = $1;

-- name: ReopenRoomForClosing :exec
-- room_started for an ended meeting: let the reconciler close it again.
UPDATE meetings SET room_closed_at = NULL, reconcile_after = NULL
WHERE id = $1 AND status = 'ended';

-- name: IsMeetingOpen :one
SELECT EXISTS (
  SELECT 1 FROM meetings WHERE id = $1 AND status = 'active' AND expires_at > now()
) AS open;

-- name: ExpireMeetings :many
UPDATE meetings SET status = 'ended', ended_at = now()
WHERE id IN (
  SELECT id FROM meetings
  WHERE status = 'active' AND expires_at <= now()
  ORDER BY expires_at LIMIT 100
  FOR UPDATE SKIP LOCKED
)
RETURNING id;

-- name: ClaimRoomsToClose :many
UPDATE meetings SET reconcile_after = now() + interval '1 minute'
WHERE id IN (
  SELECT id FROM meetings
  WHERE status = 'ended' AND room_closed_at IS NULL
    AND (reconcile_after IS NULL OR reconcile_after < now())
  LIMIT 50
  FOR UPDATE SKIP LOCKED
)
RETURNING id;

-- name: ClaimRoomsToSweep :many
UPDATE meetings SET sweep_after = now() + interval '5 minutes'
WHERE id IN (
  SELECT id FROM meetings
  WHERE room_created_at IS NOT NULL AND status = 'active'
    AND (sweep_after IS NULL OR sweep_after < now())
  LIMIT 50
  FOR UPDATE SKIP LOCKED
)
RETURNING id;

-- name: PurgeEndedMeetings :execrows
DELETE FROM meetings
WHERE id IN (
  SELECT id FROM meetings
  WHERE status = 'ended' AND ended_at < now() - sqlc.arg(retention)::interval
    AND room_closed_at IS NOT NULL
  LIMIT 500
);

-- name: CountActiveMeetings :one
SELECT count(*) FROM meetings WHERE status = 'active' AND expires_at > now();
