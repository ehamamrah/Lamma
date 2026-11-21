-- Session read model owned by the access module (PLAN.md §6): one round trip
-- returns the principal and the meeting state needed for authorization.

-- name: ResolveSession :one
SELECT p.id            AS participant_id,
       p.meeting_id,
       p.display_name,
       p.role,
       p.status,
       m.code,
       m.status        AS meeting_status,
       m.expires_at,
       m.locked,
       m.waiting_room
FROM participants p
JOIN meetings m ON m.id = p.meeting_id
WHERE p.session_hash = $1;
