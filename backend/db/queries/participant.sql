-- name: LockMeeting :one
-- Serialises joins / seat claims per meeting (PLAN.md §6 Transactions).
SELECT id, max_participants, locked, waiting_room
FROM meetings
WHERE id = $1 AND status = 'active' AND expires_at > now()
FOR UPDATE;

-- name: InsertParticipant :one
INSERT INTO participants (id, meeting_id, display_name, role, status, origin_hash, session_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetParticipant :one
SELECT * FROM participants WHERE id = $1 AND meeting_id = $2;

-- name: ParticipantExists :one
SELECT EXISTS (SELECT 1 FROM participants WHERE id = $1 AND meeting_id = $2) AS found;

-- name: GetHostID :one
SELECT id FROM participants WHERE meeting_id = $1 AND role = 'host' LIMIT 1;

-- name: CountAdmittedGuests :one
-- Only recently active rows count, so idle scripted joins age out instead of
-- permanently filling the cap (review SP4 re-verification).
SELECT count(*) FROM participants
WHERE meeting_id = $1 AND role = 'guest' AND status = 'admitted'
  AND (in_room OR lease_until > now() OR status_at > now() - interval '30 minutes');

-- name: RemoveGuest :one
-- Single statement: guarded on role and status, returns the previous status.
WITH prev AS (
  SELECT t.id, t.status FROM participants t
  WHERE t.id = sqlc.arg(id) AND t.meeting_id = sqlc.arg(meeting_id) AND t.role = 'guest'
    AND t.status IN ('admitted', 'waiting', 'left')
  FOR UPDATE
)
UPDATE participants SET status = 'removed', status_at = now()
FROM prev WHERE participants.id = prev.id
RETURNING prev.status AS previous_status;

-- name: SetParticipantStatus :one
UPDATE participants SET status = sqlc.arg(status), status_at = now()
WHERE id = $1 AND meeting_id = $2 AND status::text = ANY (sqlc.arg(from_statuses)::text[])
RETURNING *;

-- name: CountFreshWaiting :one
SELECT count(*) FILTER (WHERE origin_hash = sqlc.arg(origin_hash)) AS by_origin,
       count(*) AS total
FROM participants
WHERE meeting_id = $1 AND status = 'waiting' AND status_at > now() - interval '10 minutes';

-- name: ListFreshWaiting :many
SELECT * FROM participants
WHERE meeting_id = $1 AND status = 'waiting' AND status_at > now() - interval '10 minutes'
ORDER BY status_at
LIMIT 100;

-- name: DenyAllWaiting :many
UPDATE participants SET status = 'denied', status_at = now()
WHERE meeting_id = $1 AND status = 'waiting'
RETURNING id;

-- name: CountSeats :one
-- A seat is held by admitted guests that are connected or hold a live lease.
-- Hosts are exempt (PLAN.md §6 Seats).
SELECT count(*) FILTER (WHERE TRUE) AS total,
       count(*) FILTER (WHERE origin_hash = sqlc.arg(origin_hash) AND NOT in_room) AS unconnected_by_origin
FROM participants
WHERE meeting_id = $1 AND id <> sqlc.arg(exclude_id) AND role = 'guest'
  AND status = 'admitted' AND (in_room OR lease_until > now());

-- name: ClaimLease :one
-- lease_count counts leases taken without connecting inside a sliding window:
-- it restarts once the previous lease is long expired, so it can never lock a
-- participant out for the meeting's lifetime (review SP4-1).
UPDATE participants
SET lease_count = CASE
      WHEN in_room THEN 0
      WHEN lease_until IS NULL OR lease_until < now() - interval '10 minutes' THEN 1
      ELSE lease_count + 1
    END,
    lease_until = now() + sqlc.arg(ttl)::interval
WHERE id = $1 AND status = 'admitted'
RETURNING lease_count, in_room;

-- name: ReleaseLease :exec
UPDATE participants SET lease_until = now() WHERE id = $1;

-- name: AnyoneInRoom :one
SELECT EXISTS (SELECT 1 FROM participants WHERE meeting_id = $1 AND in_room) AS in_room;

-- name: MarkJoined :one
-- Webhook participant_joined: always record presence (a fact), ordered by event
-- time, and only for the room the participant belongs to.
UPDATE participants
SET in_room = true, in_room_at = sqlc.arg(event_at), livekit_sid = sqlc.arg(sid), lease_count = 0
WHERE id = $1 AND meeting_id = sqlc.arg(meeting_id)
  AND (in_room_at IS NULL OR in_room_at <= sqlc.arg(event_at))
RETURNING status;

-- name: MarkLeft :execrows
UPDATE participants SET in_room = false, in_room_at = sqlc.arg(event_at)
WHERE id = $1 AND livekit_sid = sqlc.arg(sid)
  AND (in_room_at IS NULL OR in_room_at <= sqlc.arg(event_at));

-- name: MarkRoomEmpty :exec
UPDATE participants SET in_room = false WHERE meeting_id = $1 AND in_room;

-- name: MarkNotInRoom :exec
UPDATE participants SET in_room = false WHERE id = $1;

-- name: ClaimIntruders :many
UPDATE participants SET reconcile_after = now() + interval '1 minute'
WHERE id IN (
  SELECT id FROM participants
  WHERE in_room AND status <> 'admitted'
    AND (reconcile_after IS NULL OR reconcile_after < now())
  LIMIT 50
  FOR UPDATE SKIP LOCKED
)
RETURNING id, meeting_id;

-- name: FilterAdmitted :many
-- Of the identities LiveKit reports as connected, which are admitted? Bounded
-- by the room size, unlike listing every admitted row.
SELECT id FROM participants
WHERE meeting_id = $1 AND status = 'admitted' AND id = ANY (sqlc.arg(candidates)::uuid[]);

-- name: CountInRoom :one
SELECT count(*) FROM participants WHERE in_room;

-- name: InsertParticipantEvent :exec
INSERT INTO moderation_events (meeting_id, actor_id, action, target_id)
VALUES ($1, $2, $3, $4);

-- name: ListInRoomIDs :many
-- Grace period: a participant_joined arriving during a sweep is not a ghost.
SELECT id FROM participants
WHERE meeting_id = $1 AND in_room AND in_room_at < now() - interval '1 minute';
