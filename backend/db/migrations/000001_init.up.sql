-- Lamma initial schema. See PLAN.md §6 for the rationale behind each column.

CREATE TYPE meeting_status AS ENUM ('active', 'ended');

CREATE TABLE meetings (
  id               uuid PRIMARY KEY,
  code             text NOT NULL UNIQUE CHECK (code ~ '^[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{4}$'),
  title            text CHECK (char_length(title) <= 120),
  password_hash    text,
  waiting_room     boolean NOT NULL DEFAULT false,
  locked           boolean NOT NULL DEFAULT false,
  status           meeting_status NOT NULL DEFAULT 'active',
  max_participants int NOT NULL CHECK (max_participants BETWEEN 2 AND 500),
  created_at       timestamptz NOT NULL DEFAULT now(),
  expires_at       timestamptz NOT NULL,
  ended_at         timestamptz,
  room_created_at  timestamptz,
  room_closed_at   timestamptz,
  reconcile_after  timestamptz,
  sweep_after      timestamptz
);

CREATE INDEX meetings_expiry_active ON meetings (expires_at) WHERE status = 'active';
CREATE INDEX meetings_ended_at ON meetings (ended_at) WHERE status = 'ended';
CREATE INDEX meetings_room_open ON meetings (reconcile_after)
  WHERE status = 'ended' AND room_closed_at IS NULL;
CREATE INDEX meetings_room_live ON meetings (sweep_after)
  WHERE room_created_at IS NOT NULL;

CREATE TYPE participant_role AS ENUM ('host', 'guest');
CREATE TYPE participant_status AS ENUM ('waiting', 'admitted', 'denied', 'removed', 'left');

CREATE TABLE participants (
  id               uuid PRIMARY KEY,
  meeting_id       uuid NOT NULL REFERENCES meetings (id) ON DELETE CASCADE,
  display_name     text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 40),
  role             participant_role NOT NULL,
  status           participant_status NOT NULL,
  status_at        timestamptz NOT NULL DEFAULT now(),
  in_room          boolean NOT NULL DEFAULT false,
  in_room_at       timestamptz,
  livekit_sid      text,
  lease_until      timestamptz,
  lease_count      smallint NOT NULL DEFAULT 0,
  origin_hash      bytea NOT NULL,
  reconcile_after  timestamptz,
  session_hash     bytea NOT NULL UNIQUE,
  created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX participants_meeting_status ON participants (meeting_id, status);
CREATE INDEX participants_unauthorized_in_room ON participants (reconcile_after)
  WHERE in_room AND status <> 'admitted';

CREATE TABLE moderation_events (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  meeting_id  uuid NOT NULL REFERENCES meetings (id) ON DELETE CASCADE,
  actor_id    uuid REFERENCES participants (id) ON DELETE CASCADE,
  action      text NOT NULL,
  target_id   uuid REFERENCES participants (id) ON DELETE CASCADE,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX moderation_events_meeting ON moderation_events (meeting_id, created_at);
