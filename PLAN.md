# Lamma — Lightweight Video Conferencing Platform (MVP)

> Phase 1 deliverable: architecture, domain boundaries, schema, API contracts,
> project structure, decisions and the delivery plan. No implementation begins
> until this document has been reviewed by both sub-agents
> (`backend-architect`, `security-performance-reviewer`) and critical findings
> are resolved.

---

## 1. Goals and Non-Goals

### Goals

- Anyone can create a meeting instantly, without an account, and share a URL.
- Real-time audio/video, screen share, chat and host moderation via **LiveKit**.
- Host privileges enforced **server-side only**; secrets never reach the browser.
- Horizontally scalable stateless API (all shared state in Postgres/Redis).
- One-command local start: `docker compose up --build`.

### Non-Goals (MVP)

- User accounts, SSO, persistent identities, contacts.
- Recording, transcription, breakout rooms, dial-in (SIP).
- Chat history persistence beyond the meeting lifetime.
- Host transfer / co-hosts (schema allows it later; see §14).
- Multi-region deployment, Kubernetes manifests.
- Custom WebRTC/SFU, WebSocket media — **never**.

---

## 2. Assumptions (documented per rule 17)

| # | Ambiguity | Decision (simplest secure option) |
| --- | --- | --- |
| A1 | What "expiration" means | `expires_at` bounds the meeting's lifetime. At expiry the meeting is ended and the LiveKit room is deleted. Default 24h, max 7d (env). |
| A2 | How a host proves identity without accounts | An opaque, high-entropy **session token** in an `HttpOnly` cookie, scoped to the meeting path. Host = a participant session with `role=host`. |
| A3 | Removed participants rejoining | Removal kills the session and LiveKit is guarded by webhook checks, but an anonymous user can always start a **new** session. The host can **lock** or enable the **waiting room** to stop re-entry. Documented limitation. |
| A4 | Chat transport | Messages go through the backend (rate-limited, validated) and are fanned out via LiveKit server `SendData`. Clients get **no** `canPublishData` grant, so chat cannot be spoofed or flood the room. |
| A5 | Chat history | Last 100 messages kept in Redis (TTL = meeting expiry) so late joiners see context. Not persisted to Postgres. |
| A6 | Muting | Host can **mute** (server-side `MutePublishedTrack`). Host can **never unmute** others (privacy). |
| A7 | Participant limits | Global LiveKit room limit + backend admission check (default 50/meeting). |
| A8 | CAPTCHA | **Deferred.** Rate limits, the argon2 gate and seat leases cover MVP abuse. The join/create handlers keep a single hook point for a future challenge verifier. |
| A9 | Host leaving | Meeting continues. `/leave` keeps the session cookie, and `/join` with a valid session resumes the same principal, so the host returns as host from the same browser. No host = no moderation until rejoin. |
| A10 | Meeting ID in logs | Logs carry the **internal meeting UUID**, never the public meeting code (the code is a capability: knowing it grants entry). |

---

## 3. High-Level Architecture

```text
Browser (React SPA)
   │ HTTPS (REST, SSE)                 │ WSS signaling + UDP/TCP/TURN media
   ▼                                   ▼
Nginx ── /api/* ──► Go API (N replicas) ◄── webhooks ── LiveKit Server (SFU+TURN)
   │                   │        │                            ▲
   └── static SPA      ▼        ▼                            │
                  PostgreSQL   Redis          Server API (Twirp) via circuit breaker
```

- **Media never touches the Go API.** The API only issues short-lived LiveKit
  JWTs and calls LiveKit's server API for moderation.
- **Go API is stateless.** Any replica can serve any request. Cross-instance
  coordination: Redis (rate limits, pub/sub for waiting-room events, chat
  buffer) and Postgres (`FOR UPDATE SKIP LOCKED` for the cleanup job).
- **Postgres is the source of truth**; Redis holds only derivable/ephemeral data.

---

## 4. Domain Model and Module Boundaries

Modular monolith: one binary, one Go module, packages per business module.
Within each module, layers are **files**, not directory trees (pragmatic Clean
Architecture: fewer packages, same dependency direction).

| Module | Responsibility | Owns tables/keys | Depends on (interfaces) |
| --- | --- | --- | --- |
| `meeting` | Create, lookup, settings (lock, waiting room, password), end, expiry | `meetings` | `MeetingStore`, `RoomCloser`, `Clock` |
| `participant` | Join, sessions, admission (waiting room), leave, token issuance | `participants` | `ParticipantStore`, `TokenIssuer`, `AdmissionNotifier` |
| `access` (Permissions) | Pure policy `Can(role, action)`; session → principal read model + middleware | session-resolve query (read-only) | `SessionResolver` |
| `moderation` | Table-less orchestrator: access check → owning module's service → LiveKit | — | `ParticipantModerator`, `MeetingModerator`, `RoomModerator` |
| `chat` | Validate, rate-limit, broadcast, recent history | Redis `chat:*` | `Broadcaster`, `ChatBuffer` |
| `realtime` | **Adapter** for LiveKit: token signing, RoomService client, webhook verification | — | LiveKit SDK, breaker, retry |
| `health` | Liveness/readiness | — | `Pinger` list |
| `platform/*` | Config, logging, Postgres, Redis, HTTP middleware, metrics, tracing, rate limiter, resilience | — | — |

### Dependency rule

```text
transport (http.go) ──► service (use cases) ──► domain (entities, errors, ports)
                                   ▲
infrastructure (postgres.go, livekit adapter) ── implements ports
```

- Domain and service code import no `net/http`, `pgx`, `redis` or LiveKit types.
- Interfaces are declared **by the consumer** (in the module that uses them),
  and only where a real boundary exists (DB, Redis, LiveKit, clock, random).
- Wiring happens once in `internal/app` via constructors. No DI framework,
  no globals, no `init()` side effects.

### Core entities

- **Meeting**: `id (uuid)`, `code` (public), `title`, `passwordHash?`,
  `waitingRoom`, `locked`, `status (active|ended)`, `maxParticipants`,
  `createdAt`, `expiresAt`, `endedAt?`.
- **Participant**: `id (uuidv7)`, `meetingID`, `displayName`,
  `role (host|guest)`, `status (waiting|admitted|denied|removed|left)`,
  `inRoom` (from webhooks), `sessionHash`, timestamps.
- **Principal** (request-scoped): `{meetingID, participantID, role, status}`
  resolved from the session cookie — the *only* input to authorization.

### Cross-module communication (no event bus)

Modules call each other through small interfaces declared by the caller.
Side effects that must reach other replicas (waiting-room decisions) go through
a direct `AdmissionNotifier` call **after commit** (Redis pub/sub). Metrics are
incremented inline. The `moderation_events` audit table is append-only and is
written by the module that owns the state change, in the same statement (ADR-012).

---

## 5. Identifiers, Credentials and Tokens

| Item | Generation | Storage | Lifetime |
| --- | --- | --- | --- |
| Meeting code | `crypto/rand`, 12 chars from a 32-char unambiguous alphabet (60 bits), shown as `xxxx-xxxx-xxxx` | `meetings.code` (unique) | Meeting lifetime |
| Meeting UUID | `uuid.New()` (v4) | `meetings.id`; LiveKit room name | Meeting lifetime |
| Session token | 32 bytes `crypto/rand`, base64url | **SHA-256 hash** in `participants.session_hash`; raw only in cookie | Meeting lifetime |
| Meeting password | user-chosen, 8–64 chars | **argon2id** (m=19 MiB, t=2, p=1) behind a concurrency gate (§11) | Meeting lifetime |
| LiveKit JWT | Signed server-side with API secret | Never stored | **2 min** = seat lease (LiveKit refreshes automatically while connected) |

Anti-enumeration: 60-bit codes + per-IP lookup limits (30/min) make guessing
infeasible (with 10⁶ live meetings, P(hit) ≈ 10⁻¹² per guess). Unknown,
expired and ended meetings all return the **same** `404`.

Session cookie:

```text
Set-Cookie: __Secure-lamma_sid=<token>; Path=/api/v1/meetings/<code>;
            HttpOnly; Secure; SameSite=Strict; Max-Age=<until expiry>
(dev, COOKIE_SECURE=false:  lamma_sid=<token>; same attributes minus Secure)
```

- Path scoping lets one browser hold independent sessions in many meetings.
- `SameSite=Strict` + `Origin` check middleware on unsafe methods = CSRF defense.
- SHA-256 (not argon2) for session hashes is correct: tokens are 256-bit random.
- With `COOKIE_SECURE=true` (required in production) the name becomes
  `__Secure-lamma_sid`. Dev runs over plain `http://localhost`, so it uses
  `COOKIE_SECURE=false` (Safari rejects `Secure` cookies on http).
- Sessions are never cleared on leave; only removal, denial or meeting end kill them.

LiveKit grants (identical for host and guest — host powers live in the API):
`roomJoin`, `room=<meeting uuid>`, `canPublish`, `canSubscribe`,
`canPublishSources=[camera, microphone, screen_share, screen_share_audio]`,
`canPublishData=false`, `canUpdateOwnMetadata=false`, `hidden=false`.
Server sets participant `metadata={"role":"host|guest"}` for **display only**.

---

## 6. Database Schema (PostgreSQL 17)

```sql
CREATE TYPE meeting_status AS ENUM ('active', 'ended');
CREATE TABLE meetings (
  id               uuid PRIMARY KEY,
  code             text NOT NULL UNIQUE,   -- canonical: xxxx-xxxx-xxxx lowercase
  title            text CHECK (char_length(title) <= 120),
  password_hash    text,
  waiting_room     boolean NOT NULL DEFAULT false, locked boolean NOT NULL DEFAULT false,
  status           meeting_status NOT NULL DEFAULT 'active',
  max_participants int NOT NULL CHECK (max_participants BETWEEN 2 AND 500),
  created_at       timestamptz NOT NULL DEFAULT now(),
  expires_at       timestamptz NOT NULL, ended_at timestamptz,
  room_created_at  timestamptz,          -- LiveKit room exists (cleared on room_finished)
  room_closed_at   timestamptz,          -- LiveKit DeleteRoom confirmed
  reconcile_after  timestamptz, sweep_after timestamptz  -- worker claims / backoff
);
```

```sql
CREATE INDEX meetings_expiry_active ON meetings (expires_at) WHERE status = 'active';
CREATE INDEX meetings_ended_at ON meetings (ended_at) WHERE status = 'ended';
CREATE INDEX meetings_room_open ON meetings (reconcile_after)
  WHERE status = 'ended' AND room_closed_at IS NULL;
```

```sql
CREATE TYPE participant_role AS ENUM ('host', 'guest');
CREATE TYPE participant_status AS ENUM ('waiting','admitted','denied','removed','left');
CREATE TABLE participants (
  id            uuid PRIMARY KEY,   -- uuidv7; also the LiveKit identity
  meeting_id    uuid NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
  display_name  text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 40),
  role          participant_role NOT NULL, status participant_status NOT NULL,
  in_room       boolean NOT NULL DEFAULT false,
  in_room_at    timestamptz, livekit_sid text,  -- last applied webhook event + SID
  lease_until   timestamptz,        -- seat reservation before LiveKit connect
  lease_count   smallint NOT NULL DEFAULT 0, -- leases since last participant_joined
  origin_hash   bytea NOT NULL,     -- HMAC(client IP or IPv6 /64); no raw IPs
  reconcile_after timestamptz,      -- reconciler claim / backoff
  session_hash  bytea NOT NULL UNIQUE, created_at timestamptz NOT NULL DEFAULT now()
);
```

```sql
CREATE INDEX participants_meeting_status ON participants (meeting_id, status);
CREATE INDEX participants_unauthorized_in_room ON participants (reconcile_after)
  WHERE in_room AND status <> 'admitted';
CREATE TABLE moderation_events (           -- append-only audit log
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  meeting_id  uuid NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
  actor_id    uuid NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
  action      text NOT NULL,  -- remove|mute|admit|deny|lock|unlock|end|...
  target_id   uuid REFERENCES participants(id) ON DELETE CASCADE,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX moderation_events_meeting ON moderation_events (meeting_id, created_at);
```

### Seats (capacity model)

A participant **holds a seat** when `status='admitted' AND (in_room OR
lease_until > now())`. Seats are claimed at **token issuance**, not at join or
admit, so closed tabs and scripted joins cannot hold capacity:

- `/token` claims (or renews) a lease equal to the LiveKit token TTL (2 min)
  inside the capacity transaction, so a token can never outlive its seat.
- At most **3 leased-but-unconnected seats per `origin_hash`** per meeting, so
  one IP cannot fill a meeting by minting sessions.
- A participant may hold at most **3 leases without connecting** (`lease_count`,
  reset by `participant_joined`), so a script cannot squat a seat by renewing.
- The **host is exempt** from the seat count (a host can always rejoin to
  moderate); LiveKit's `max_participants` is set to `max + 1` and is the hard cap.
- `participant_joined` webhook sets `in_room=true`; the lease no longer matters.
- `participant_left` / `room_finished` sets `in_room=false`; the seat frees.

### Query patterns → indexes

| Hot query | Index used |
| --- | --- |
| Resolve session (every authenticated request) | `participants.session_hash` unique |
| Lookup meeting by code (join, public info) | `meetings.code` unique |
| Count seats at token issuance | `participants_meeting_status` |
| List / count waiting participants | `participants_meeting_status` |
| Expire meetings batch | `meetings_expiry_active` (partial) |
| Reconcile rooms still open after end | `meetings_room_open` (partial) |
| Reconcile connected-but-not-admitted participants | `participants_unauthorized_in_room` (partial) |
| Purge ended meetings after retention | `meetings_ended_at` (partial) |

### Transactions and concurrency

- **Seat claim**: `SELECT … FROM meetings WHERE id=$1 FOR UPDATE`, count seats
  excluding the caller, set caller's `lease_until` — one transaction, so
  concurrent token requests cannot exceed `max_participants`.
- **Liveness guard**: every guard query includes `status='active' AND
  expires_at > now()` — expiry is effective immediately, not when the worker runs.
- **Session resolve** is a read model owned by `access`: one query joining
  `participants ⋈ meetings` returns the principal + meeting state (no N+1).
- **State change + audit** happen in the owning module's single statement/tx
  (`WITH u AS (UPDATE … RETURNING …) INSERT INTO moderation_events …`).
- **Workers** claim rows without holding transactions across network calls:
  `UPDATE … SET reconcile_after = now() + '1 min' WHERE id IN (SELECT … WHERE
  reconcile_after IS NULL OR reconcile_after < now() LIMIT 50 FOR UPDATE SKIP
  LOCKED) RETURNING …` — one replica per row, automatic retry after 1 min.
- **Seat-claim tx commits before any LiveKit call** — no row lock or pool
  connection is held across the network.
- **Retention**: ended meetings hard-deleted after `MEETING_RETENTION` (default 7d), cascading.

SQL lives in `backend/db/queries/<module>.sql`; **sqlc** generates one package
per module (`internal/<module>/store`), so a module cannot call another
module's queries. Migrations via **golang-migrate** (`backend/db/migrations`),
run by a one-shot `migrate` container before the API starts.

---

## 7. Redis Usage (never the source of truth)

| Key pattern | Purpose | TTL |
| --- | --- | --- |
| `rl:{policy}:{scope}` | GCRA rate-limit state (`go-redis/redis_rate`) | Auto (window) |
| `chat:{meeting_uuid}` | Capped list (`LPUSH` + `LTRIM 0 99`) of recent chat | Until `expires_at` |
| `admission:{participant_id}` | Pub/sub: admit/deny for one waiting participant | n/a |

- One `PSUBSCRIBE admission:*` connection **per replica** feeds an in-process
  hub (map participant → channel). SSE streams never hold their own Redis
  connection.
- Events are published **only after the DB transaction commits**.

Redis outage policy:

- Rate limiting: **fail-closed** for meeting creation and password attempts;
  **fail-open** for everything else, degraded to a coarse per-replica
  in-memory limiter per policy (`golang.org/x/time/rate`).
- Chat history unavailable → chat send still works (broadcast is LiveKit).
- SSE: clients reconnect; the DB status check on connect still delivers decisions.
- Readiness does **not** fail on Redis (that would drain every replica at
  once); Redis state is reported in the readiness body and in metrics.

---

## 8. API Contract (v1)

Base path `/api/v1`. JSON only. Errors use **RFC 9457** `application/problem+json`
with a stable `code` field, never internal details. Full spec: `api/openapi.yaml`.
Auth column: `—` public, `S` any valid session, `H` host session.
`{code}` must be canonical (`xxxx-xxxx-xxxx`, lowercase); the SPA normalises
user input and redirects before calling the API, so the cookie path always matches.

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| POST | `/meetings` | — | Create meeting; caller becomes host (sets cookie) |
| GET | `/meetings/{code}` | — | Public info: `title, requiresPassword, waitingRoom, locked` |
| POST | `/meetings/{code}/join` | — / S | Join with `displayName`, `password?`; **idempotent** with a valid session |
| GET | `/meetings/{code}/me` | S | Current principal (`participantId, role, status`) |
| GET | `/meetings/{code}/admission/events` | S (waiting) | **SSE**: `admitted` / `denied` |
| POST | `/meetings/{code}/token` | S (admitted) | Claim seat, ensure room, return `{token, url, expiresAt}` |
| POST | `/meetings/{code}/leave` | S | Release seat; session is kept for rejoin |
| GET | `/meetings/{code}/chat` | S (admitted) | Recent chat messages |
| POST | `/meetings/{code}/chat` | S (admitted) | Send chat `{text ≤ 1000}` |
| GET | `/meetings/{code}/participants?status=waiting` | H | Waiting list (max 100) |
| POST | `/meetings/{code}/participants/{id}/admit` | H | Admit from waiting room |
| POST | `/meetings/{code}/participants/{id}/deny` | H | Deny |
| POST | `/meetings/{code}/participants/deny-waiting` | H | Deny everyone waiting |
| POST | `/meetings/{code}/participants/{id}/remove` | H | Kick + revoke session |
| POST | `/meetings/{code}/participants/{id}/mute` | H | `{source: microphone|camera|screen_share}` |
| PATCH | `/meetings/{code}/settings` | H | `{locked?, waitingRoom?}` |
| POST | `/meetings/{code}/end` | H | End for all |
| POST | `/internal/livekit/webhook` | LiveKit signature | Not routed by Nginx |
| GET | `/health/live`, `/health/ready` | — | Probes |
| GET | `/metrics` | — | Prometheus — **separate internal port**, not proxied |

Example — create:

```http
POST /api/v1/meetings
{"displayName":"Elayan","title":"Standup","password":null,
 "waitingRoom":true,"expiresInMinutes":120}

201 Created   Set-Cookie: lamma_sid=…; Path=/api/v1/meetings/abcd-efgh-jkmn; HttpOnly; …
{"meeting":{"code":"abcd-efgh-jkmn","url":"https://host/m/abcd-efgh-jkmn",
  "expiresAt":"…","waitingRoom":true,"locked":false},
 "participant":{"id":"…","role":"host","status":"admitted"}}
```

Status codes: join `200 admitted` / `202 waiting`; `401 password_required |
invalid_password`; `403 meeting_locked | removed | denied`; `409 meeting_full |
waiting_room_full`; `404 not_found`; `429 rate_limited` (+ `Retry-After`);
`503 server_busy | livekit_unavailable` (+ `Retry-After`). Moderation returns
`204`, or `202 {"livekitPending":true}` when the DB change committed but the
LiveKit call is deferred to the reconciler.

Authorization guarantees:

- Role comes **only** from the session row. Request bodies never carry roles.
- Every host route runs `RequireRole(host)` middleware **and** the service
  re-checks `access.Can(principal, action)` — defense in depth.
- Target participant must belong to the principal's meeting (IDOR check in SQL:
  `WHERE id=$1 AND meeting_id=$2`).
- `removed` / `denied` sessions are dead; ended or expired meetings reject
  everything except `GET /me`.
- LiveKit itself is guarded: rooms are never auto-created, and every
  `participant_joined` webhook is checked against the DB (§9).

---

## 9. Key Flows

**Create** → rate limit → validate → hash password (argon2 gate, §11) →
generate code (retry on unique collision, max 3) → one tx: meeting + host
participant (`admitted`) → set cookie → 201. No LiveKit call.

**Join** (no session) → rate limits (IP, IP+meeting, meeting) → load meeting
→ `404` if ended/expired → `403` if locked → password check (rate limit is
consumed **before** argon2 runs) → waiting room on? insert `waiting` (caps: 3
active waiters per origin and 50 per meeting, else `409 waiting_room_full`) and
notify host; otherwise insert `admitted` → set cookie. Hosts also get a
**deny-all-waiting** action.

**Join** (valid session for this meeting) — idempotent resume, exempt from the
per-meeting join limit, no password: host → `admitted`; guest `admitted` or
fresh `waiting` → unchanged; stale `waiting` (> 10 min) → treated as a new
waiting entry (caps below apply, clock is not silently refreshed); guest `left` → re-evaluated against lock / waiting room;
`removed` / `denied` → `403`.

**Waiting room** → SSE connect → register in hub **first**, then read status
from DB; if already decided emit immediately → otherwise wait for hub event.
Heartbeat 25s, max lifetime 10 min (waiting entries older than 10 min are
ignored by host lists and caps; a stale waiter must re-enter as a new entry). Host is notified of new
waiters by LiveKit `SendData` to the host identity (best effort) and the host
UI loads the list on that signal.

**Token** → principal `admitted`, meeting active and unexpired → seat-claim tx
(`409 meeting_full`; commits) → skip `CreateRoom` only if `room_created_at`
is set **and** someone in the meeting is `in_room`; otherwise call `CreateRoom`
(idempotent; `max_participants`, `empty_timeout`) via the `livekit_room`
breaker and set `room_created_at` → sign JWT locally → return. A busy room
never touches LiveKit on the token path, and a lost `room_finished` webhook
cannot strand a meeting.

**Leave** → `lease_until = now()`; guest status → `left`; host stays
`admitted`. Cookie kept.

**Remove / Deny / Admit / Lock / End** → `moderation` checks `access.Can` and
calls the owning module's service (`participant.Remove`, `meeting.End`, …),
which writes state + audit in one statement → after commit, the LiveKit call
runs with `context.WithoutCancel(ctx)` + 3s timeout (a client disconnect can't
skip it) → `204`, or `202 livekitPending` on LiveKit failure. Admitting runs
no capacity check (capacity is enforced at token time).

**Mute** → `realtime.MuteSource(room, identity, source)` = `GetParticipant` →
find track by source → `MutePublishedTrack`; retried as one unit.

**End** → `status=ended` → `DeleteRoom` → set `room_closed_at` → delete chat key.

**Webhooks** → `webhook.ReceiveWebhookEvent` (signature + body hash) →

- `participant_joined`: **always** record `in_room=true`, `livekit_sid`,
  `in_room_at` (it is a fact). Then, if the participant is not `admitted` or
  the meeting is not active → `RemoveParticipant`. If that call fails, the
  reconciler finds the row via `in_room AND status <> 'admitted'`.
- `participant_left`: `in_room=false` only if `livekit_sid` matches and
  `event.createdAt >= in_room_at` (webhook timestamps are whole seconds).
- `room_started` for an ended meeting → clear `room_closed_at` (reconciler
  retries) and `DeleteRoom`.
- `room_finished` → `in_room=false` for the meeting; clear `room_created_at`.
- `participant_joined` applies only if `event.createdAt >= in_room_at`.

---

## 10. Resilience

### Rate limits (Redis GCRA, all env-configurable)

| Policy | Key | Default |
| --- | --- | --- |
| General API | IP | 120/min |
| Meeting create | IP | 10/hour |
| Meeting lookup | IP | 30/min |
| Join | IP / meeting | 10/min per IP; 60/min per meeting (sessionless callers only) |
| Password attempts | IP+meeting | 5 per 15 min — counted **before** verify (no meeting-wide cap) |
| Token issue | session | 6/min |
| Chat | session / meeting | 20 per 10s; 10/s per meeting; 1000 chars |
| Moderation | session | 60/min |
| SSE streams | IP / replica | 3 concurrent per IP; 2000 per replica (in-process) |

- Client IP: `X-Forwarded-For` honoured **only** when the TCP peer is in
  `TRUSTED_PROXY_CIDRS` (rightmost untrusted hop); otherwise `RemoteAddr`.
  Dev trusts only Nginx's fixed address (`172.30.0.10/32`); the API port is
  never published. In production the variable is **required**.
- IPv6 clients are limited by **/64** prefix to stop address rotation.
- Limit keys use the meeting UUID (or canonical code), never raw input.
- 429 responses include `Retry-After` and increment `ratelimit_rejections_total{policy}`.

### Circuit breakers — `sony/gobreaker/v2`

Wrap **only** LiveKit server API calls. Not DB/Redis (pools + timeouts
suffice; a breaker there hides real outages). Three breakers, so neither chat
nor token traffic can disable moderation:

| Breaker | Operations |
| --- | --- |
| `livekit_room` | `CreateRoom` (token path) |
| `livekit_admin` | `DeleteRoom`, `RemoveParticipant`, `GetParticipant`, `ListParticipants`, `MutePublishedTrack` |
| `livekit_data` | `SendData` (chat, host notifications) |

| Setting | Default |
| --- | --- |
| Trip | ≥ 5 consecutive failures, or ≥ 50% failures over ≥ 20 requests in 30s |
| Open timeout (→ half-open) | 30s |
| Half-open max probes | 3 |
| Per-call timeout | 3s |
| `IsSuccessful` | `nil`, `context.Canceled`, LiveKit `not_found` and other 4xx count as success |

State exported as `circuit_breaker_state{name}` (0 closed, 1 half-open, 2 open).
Open → `503 livekit_unavailable` + `Retry-After` for token/moderation.

### Retries — `cenkalti/backoff/v5`

- Only **idempotent** ops: `CreateRoom`, `DeleteRoom`, `RemoveParticipant`,
  `MuteSource` (lookup + mute). `SendData` is **not** retried.
- Max 3 attempts, base 100ms, cap 1s, full jitter, respects context.
- Order: `retry( breaker( call ) )`; `gobreaker.ErrOpenState` is a permanent
  error → no retry storms against an open breaker.

### Timeouts

| Layer | Value |
| --- | --- |
| HTTP server | ReadHeader 5s, Read 10s, Write 15s, Idle 60s (SSE extends via `http.ResponseController`) |
| Request body | 16 KiB (`http.MaxBytesReader`) |
| Postgres | pool: max 20, min 2, lifetime 30m, idle 5m; per-query ctx timeout 2s |
| Redis | dial 2s, read/write 500ms, pool 50 |
| LiveKit | 3s per call (above) |

### Background work (one maintenance loop per replica, 30s ticker)

1. **Expire**: active meetings past `expires_at` → `ended` (`SKIP LOCKED`, batch 100).
2. **Reconcile rooms**: ended meetings with `room_closed_at IS NULL` →
   `DeleteRoom` → set `room_closed_at` (`not_found` counts as closed).
3. **Reconcile intruders**: `in_room AND status <> 'admitted'` →
   `RemoveParticipant` (`not_found` → `in_room=false`).
4. **Purge**: ended meetings older than `MEETING_RETENTION`.
5. **Anti-entropy sweep** (every 5 min per room, claimed via `sweep_after`):
   `ListParticipants` for rooms with `room_created_at` set → remove anyone
   not `admitted` in the DB, and correct `in_room` for lost webhooks. On
   `not_found`: clear `room_created_at` and set `in_room=false` for the meeting.
6. **Gauges**: refresh `meetings_active`, `participants_in_room`.

LiveKit fan-out is bounded by a semaphore (8). State lives in Postgres, so a
crash loses nothing — there is no in-memory work queue. The loop takes the root
context and is joined with `errgroup` on shutdown.

### Graceful shutdown

`signal.NotifyContext(SIGINT, SIGTERM)` → readiness flips to 503 → wait 5s
(load balancer drains) → cancel **only the SSE hub** (`RegisterOnShutdown`), so
streams send `event: shutdown` and exit; ordinary in-flight requests finish →
`server.Shutdown` (deadline 20s) → cancel worker ctx → wait errgroup → flush
OTel → close Redis → close pgx pool. Compose `stop_grace_period: 30s`.

---

## 11. Security Design

- **Secrets**: `LIVEKIT_API_SECRET`, DB/Redis passwords only via env/secret files.
  Never in images, logs, responses or the frontend bundle. Config struct has a
  redacting `LogValue()`.
- **Authorization**: §8 guarantees. Role never accepted from client.
- **Input validation**: hand-written validators per DTO (lengths, enums, Unicode
  normalization NFC; strip control (Cc) characters and bidi controls
  U+202A–202E, U+2066–2069 — ZWJ/ZWNJ stay, so emoji and Persian names work). The UI shows the role as a separate badge, never in
  the name. Unknown
  JSON fields rejected (`DisallowUnknownFields`).
- **SQL injection**: impossible by construction — sqlc parameterized queries only.
- **CSRF**: `SameSite=Strict` cookies + `Origin`/`Sec-Fetch-Site` check on
  unsafe methods + JSON-only content type.
- **CORS**: same-origin by default (SPA served by same Nginx). Explicit
  allowlist via `CORS_ALLOWED_ORIGINS`; no wildcard with credentials.
- **Security headers** (Nginx + API): CSP (`default-src 'self'; connect-src
  'self' wss://<livekit> https://<livekit>; frame-ancestors 'none'`), HSTS (prod),
  `X-Content-Type-Options: nosniff`, **`Referrer-Policy: no-referrer`** (meeting
  URLs are capabilities), `Permissions-Policy: camera=(self), microphone=(self),
  display-capture=(self)`.
- **Argon2 gate**: argon2id costs ~19 MiB per check, so hashing/verifying runs
  behind a weighted semaphore (`ARGON2_MAX_CONCURRENT`, default `NumCPU`);
  if no slot frees within 1s the API returns `503 server_busy`. Password rate
  limits are **consumed before** verification, so parallel bursts cannot
  bypass them.
- **Brute force**: 5 attempts / 15 min per IP+meeting (IPv6 per /64), no
  meeting-wide cap or lockout (an outsider cannot lock real guests out; valid
  sessions never need the password again). 8-char minimum + argon2 cost.
  Constant-time comparison is inherent to argon2id verification.
- **Enumeration**: 60-bit codes, uniform 404, lookup rate limit.
- **Abuse**: creation limits, global per-IP limits, max participants,
  max meeting duration, seat leases (closed tabs free capacity in 2 min; max 3
  unconnected leased seats per origin per meeting), waiting-room caps (3 per
  origin, 50 per meeting), SSE caps.
- **LiveKit**: `room.auto_create: false` — rooms exist only after an API
  token request; every `participant_joined` is checked against the DB.
- **Production guards**: startup fails if `LIVEKIT_API_SECRET` is shorter than
  32 bytes or a known dev value, if `TRUSTED_PROXY_CIDRS` is unset, or if
  `COOKIE_SECURE` is false.
- **Logs**: no tokens, cookies, passwords, display names or meeting codes.
  `slog` handler with a redaction allowlist of attribute keys.
- **Containers**: non-root, read-only root FS, `no-new-privileges`, dropped caps,
  distroless/static runtime for Go.

---

## 12. Observability

- **Logging**: `log/slog` JSON. Middleware injects `request_id` (accept
  `X-Request-ID` if well-formed, else generate), `trace_id`, `meeting_id`
  (internal UUID), `participant_id`, route pattern, status, latency.
- **Tracing**: OpenTelemetry SDK, OTLP exporter **only if**
  `OTEL_EXPORTER_OTLP_ENDPOINT` is set (no-op otherwise). Spans: HTTP (otelhttp),
  pgx (tracer hook), LiveKit adapter calls.
- **Metrics** (`client_golang`, internal port `:9090/metrics`):

| Metric | Type |
| --- | --- |
| `http_requests_total{route,method,status}` | counter |
| `http_request_duration_seconds{route,method}` | histogram |
| `ratelimit_rejections_total{policy}` | counter |
| `meetings_active`, `participants_in_room` | gauge (refreshed by worker, 30s) |
| `livekit_operation_failures_total{op}` | counter |
| `circuit_breaker_state{name}` | gauge |
| `db_pool_*` (acquired, idle, total, wait duration) | gauge via collector |
| `redis_operation_duration_seconds{op}` | histogram (go-redis hook) |

Route labels use **chi route patterns**, never raw paths (cardinality).

- **Health**: `/health/live` → always 200 if process serves. `/health/ready` →
  fails only on **Postgres** (500ms timeout, cached 2s to avoid probe storms)
  or during shutdown. Redis and LiveKit state are reported in the body but do
  not fail readiness: both degrade gracefully (§7, §10), and failing readiness
  on a shared dependency would drain every replica at once.
- `meetings_active` / `participants_in_room` are global values computed by
  every replica — aggregate with `max()`, not `sum()`.

---

## 13. Project Structure

```text
backend/
├── cmd/api/main.go          # tiny: load config, app.Run(ctx)
├── internal/
│   ├── app/                 # wiring, server, worker lifecycle
│   ├── meeting/             # domain.go service.go ports.go postgres.go http.go
│   ├── participant/  access/  moderation/  chat/
│   ├── realtime/            # LiveKit adapter: tokens, rooms, webhooks
│   ├── health/
│   └── platform/            # config, logging, postgres, redis, httpx,
│                            # metrics, telemetry, ratelimit, resilience
├── db/migrations/  db/queries/   # golang-migrate + sqlc sources
├── internal/*/store/         # sqlc output per module (committed, CI checks drift)
└── sqlc.yaml  .golangci.yml  Dockerfile  go.mod
```

```text
frontend/
├── src/app/                 # router, providers, layout
├── src/features/            # create-meeting, join, prejoin, waiting,
│                            # room, chat, moderation (hooks + components)
├── src/shared/{api,ui,lib}/ # typed API client, shadcn/ui, utils
├── src/stores/              # Zustand: session, ui (not media state)
└── Dockerfile  nginx.conf  vite.config.ts
```

```text
deploy/livekit/livekit.yaml   # dev; prod template uses env substitution
deploy/nginx/                 # reverse proxy + security headers
api/openapi.yaml
docs/                         # architecture.md, adr/, security.md, ops.md
compose.yaml                  # base
compose.override.yaml         # dev (auto-loaded by `docker compose up`)
compose.prod.yaml             # prod overlay (TLS, no dev ports)
.env.example
.github/workflows/ci.yml
.claude/agents/               # backend-architect.md, security-performance-reviewer.md
```

Frontend rules: business logic in hooks/stores (`features/*/use*.ts`),
components are presentational. Media state lives in the LiveKit SDK, not
Zustand. Routes: `/` (create/join), `/m/:code` (pre-join with device preview →
waiting → room), `*` (404).

---

## 14. Architecture Decision Records (summary)

Full ADRs go in `docs/adr/NNNN-*.md` in Phase 1 review.

| ADR | Decision | Why | Alternatives rejected |
| --- | --- | --- | --- |
| 001 | Modular monolith, packages per module, layers as files | Simplest deployable unit; clear boundaries; cheap to split later | Microservices (rule 2), deep `domain/app/infra` dirs (ceremony) |
| 002 | LiveKit for all media + TURN | Proven SFU, Go SDK, webhooks | Custom SFU (rule 6) |
| 003 | Cookie session (hashed) for host/guest identity | `HttpOnly` keeps credential away from XSS; server-side revocation | JWT in localStorage (XSS-exposed, no revocation) |
| 004 | Chat through backend → LiveKit `SendData` | Server-side rate limit/validation; no client data publish | Client data channels (unenforceable limits, spoofing) |
| 005 | SSE + Redis pub/sub for waiting room | One-way push, works across replicas, plain HTTP | Polling (load), WebSocket (unneeded bidirectional) |
| 006 | Redis GCRA (`redis_rate`) | Atomic Lua, distributed, smooth bursts | Fixed window (burst at edges), in-memory (per-instance) |
| 007 | `gobreaker/v2` around LiveKit only | External network dep; DB/Redis covered by pools/timeouts | Breakers everywhere (masks outages) |
| 008 | Postgres `SKIP LOCKED` for expiry | No distributed lock service needed | Redis lock (extra failure mode) |
| 009 | Internal UUID vs public code | Logs/LiveKit never hold the access capability | Single ID |
| 010 | Stdlib-first config (`os.LookupEnv` + validation) | Avoid dependency for ~30 vars | viper/envconfig |
| 011 | Migrations run by one-shot container | Avoid replicas racing migrations | Migrate on API boot |
| 012 | `moderation` is a table-less orchestrator; owners write state + audit | Atomic state+audit without cross-module writes | Moderation writing `participants` |
| 013 | Seats claimed at token time via 2-min lease + `in_room` | Closed tabs/scripted joins can't hold capacity | Counting `admitted` rows |
| 014 | `auto_create: false` + webhook guard + DB-driven reconciler | Removed/ended users can't re-enter via refreshed tokens; nothing lost on crash | In-memory retry queue |
| 015 | Per-replica `PSUBSCRIBE` hub for SSE | O(1) Redis connections per replica | One subscription per stream |

### Dependency list (each justified)

Backend: `go-chi/chi/v5`, `jackc/pgx/v5`, `sqlc` (codegen), `redis/go-redis/v9`,
`go-redis/redis_rate/v10`, `livekit/server-sdk-go/v2` + `livekit/protocol`,
`sony/gobreaker/v2`, `cenkalti/backoff/v5`, `golang-migrate/migrate/v4`,
`google/uuid`, `golang.org/x/crypto` (argon2id), `golang.org/x/text` (NFC),
`golang.org/x/sync` (errgroup, semaphore), OpenTelemetry SDK + `otelhttp`,
`prometheus/client_golang`. Tests: `testcontainers-go`, `alicebob/miniredis`
(unit), `stretchr/testify` (assertions only).

Frontend: React 19, TypeScript, Vite, Tailwind CSS v4, shadcn/ui, Zustand,
React Router, `livekit-client`, `@livekit/components-react`, Vitest, Testing
Library, ESLint, Prettier.

---

## 15. Docker and LiveKit Networking

Services: `postgres`, `redis`, `migrate` (one-shot), `livekit`, `api`, `web`
(Nginx serving SPA + proxying `/api`). Startup order via
`depends_on: condition: service_healthy` / `service_completed_successfully`.

| Port | Proto | Service | Purpose |
| --- | --- | --- | --- |
| 8080 | TCP | web (Nginx) | SPA + `/api` (dev); 443 in prod |
| 7880 | TCP | livekit | Signaling (WS); behind TLS (`wss://`) in prod |
| 7881 | TCP | livekit | ICE/TCP fallback when UDP is blocked |
| 7882 | UDP | livekit | WebRTC media (single-port UDP mux) |
| 3478 | UDP | livekit TURN | TURN/UDP |
| 5349 | TCP | livekit TURN | TURN/TLS (prod; ideally on 443 via dedicated IP/LB) |

- **HTTP ports alone are not enough**: media needs UDP 7882 (or a port range)
  and fallbacks TCP 7881 / TURN. Firewalls/security groups must allow them.
- Local (Docker Desktop): `rtc.node_ip: 127.0.0.1`, `use_external_ip: false`.
  LAN testing from phones: set `LIVEKIT_NODE_IP` to the host's LAN IP.
- Prod: `use_external_ip: true` (STUN discovery) or explicit public IP; TURN
  with a real TLS cert for restrictive corporate networks.
- LiveKit `room.auto_create: false`, `empty_timeout: 300`, webhooks to
  `http://api:8000/internal/livekit/webhook` (internal network only).
- Nginx has a fixed address (`172.30.0.10`) on the `lamma` network; the API
  trusts `X-Forwarded-For` only from it. The API port is **not** published.
  SSE location: `proxy_buffering off; proxy_read_timeout 60s;`;
  `worker_connections 4096`.
- Prod behind a load balancer: Nginx uses `set_real_ip_from <LB CIDR>;
  real_ip_header X-Forwarded-For; real_ip_recursive on;` and forwards the
  recovered address, so per-IP limits see real clients.
- TURN relay must not reach internal addresses: verify LiveKit's TURN peer
  restrictions in Phase 2 and block private ranges at the host firewall in prod.
- Containers: multi-stage builds; Go → `gcr.io/distroless/static:nonroot`;
  web → `nginxinc/nginx-unprivileged:alpine`; healthchecks on every service;
  named volume for Postgres; images pinned by digest in prod overlay.

Video defaults: simulcast on, adaptive stream + dynacast on, camera 720p capture
with 3 simulcast layers, screen share 1080p/15fps.

---

## 16. Testing Strategy

| Level | What | How |
| --- | --- | --- |
| Unit | meeting creation/validation, expiry, `access.Can` matrix, host authz in services, token claims/TTL, code generator entropy/format, rate-limit policy selection, breaker state transitions, retry classification | Table-driven, fakes for ports, fake clock |
| Integration | sqlc repositories, admission capacity under concurrency, `SKIP LOCKED` expiry with 2 workers, Redis GCRA limits, pub/sub SSE, LiveKit webhook verification, LiveKit adapter against a real `livekit-server --dev` container | `testcontainers-go`, build tag `integration` |
| Concurrency | N goroutines joining a meeting at capacity; parallel admit/deny; concurrent rate-limit hits | `-race`, `t.Parallel`, `errgroup` |
| Frontend | stores, API client, hooks, key components | Vitest + Testing Library |
| E2E | create → join (2 browsers) → media connected → chat → remove → end | Playwright with fake media devices (Phase 6) |

CI always runs `go test -race ./...`.

---

## 17. CI/CD (`.github/workflows/ci.yml`)

Actions are pinned by commit SHA and tool versions are pinned.

1. **backend-lint**: gofmt, golangci-lint v2.15 (`funlen` set to 15
   statements, gocognit, nestif, gosec, …) and a check that the sqlc
   output is up to date.
2. **backend-test**: `go test -race -cover ./...`.
3. **frontend**: Prettier, ESLint, `tsc`, Vitest.
4. **stack**: `docker compose up --build`, then the Postgres integration
   tests, the API e2e tests and the Playwright browser e2e (two
   participants, fake media).
5. **security**: govulncheck, npm audit, Trivy filesystem scan, Gitleaks.
6. **images**: build both images and scan them with Trivy.

---

## 18. Delivery Plan and Review Gates

Each phase ends with **both** sub-agents run in parallel on the phase's diff.
Gate: zero unresolved **Critical/High** findings; Medium findings either fixed
or recorded in `docs/review-log.md` with justification.

| Phase | Scope | Exit criteria |
| --- | --- | --- |
| 1. Architecture | This plan, ADRs, sub-agents, OpenAPI skeleton | Plan reviewed by both agents; findings resolved |
| 2. Infrastructure | Dockerfiles, compose (base/dev/prod), Postgres, Redis, LiveKit, Nginx, `.env.example` | `docker compose up --build` → all healthchecks green |
| 3. Backend foundation | Go module, config, slog, pgx, Redis, migrations, middleware (request ID, recover, logging, body limit, headers, CORS, real IP), health, metrics, OTel | `/health/ready` 200; lint + tests green |
| 4. Business logic | Meetings, participants/sessions, access, moderation, chat, LiveKit adapter + breaker/retry, rate limiting, workers, webhooks | Unit + integration tests; OpenAPI complete |
| 5. Frontend | Landing, create, join, device preview, waiting room, room UI, controls, screen share, chat, participant list, host moderation | Two browsers can meet end-to-end |
| 6. Quality | Concurrency tests, E2E, security & performance review, Docker validation, docs sync | Definition of Done met |

### Definition of Done (traceability)

| Requirement | Verified by |
| --- | --- |
| Create meeting without registration | E2E + `POST /meetings` tests |
| Share URL, others join by name | E2E two-browser |
| Multi-party audio/video, screen share | E2E with fake devices + manual check |
| Permissions enforced server-side | Unit authz matrix + integration IDOR tests |
| Rate limiting across instances | `TestLimitIsSharedAcrossInstances` (two limiters, one Redis) |
| Graceful handling of dependency failure | Breaker tests; LiveKit-down unit test (`participant/service_test.go`); degraded limiter tests |
| Starts via Docker Compose | CI docker job + compose smoke test |
| Automated tests pass | CI green |
| No critical security issues | `security-performance-reviewer` sign-off |

---

## 19. Environment Variables (reference; full list in `.env.example`)

| Variable | Default | Notes |
| --- | --- | --- |
| `APP_ENV` | `production` (binary), `development` (compose) | `production` enables HSTS and the production guards (§11) |
| `COOKIE_SECURE` | `false` (dev) | Must be `true` in production |
| `ARGON2_MAX_CONCURRENT` | `NumCPU` | Concurrent password hash/verify slots |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Used to build meeting URLs |
| `HTTP_ADDR` / `METRICS_ADDR` | `:8000` / `:9090` | Metrics port is internal-only |
| `DATABASE_URL` | — | Required |
| `DB_MAX_CONNS` | `20` | pgx pool |
| `REDIS_URL` | — | Required |
| `LIVEKIT_URL` / `LIVEKIT_PUBLIC_URL` | — | Internal API URL / browser `wss://` URL |
| `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET` | — | Secret: server only |
| `LIVEKIT_TOKEN_TTL` | `2m` | Also the seat lease |
| `ORIGIN_HASH_KEY` | — | Secret HMAC key for `origin_hash` (≥ 32 bytes in production) |
| `MEETING_DEFAULT_TTL` / `MEETING_MAX_TTL` | `24h` / `168h` | |
| `MEETING_MAX_PARTICIPANTS` | `50` | |
| `MEETING_RETENTION` | `168h` | Hard-delete ended meetings after |
| `RATE_LIMIT_*` | see §10 | One var per policy, e.g. `RATE_LIMIT_CREATE=10/h` |
| `TRUSTED_PROXY_CIDRS` | `172.30.0.10/32` (dev) | Nginx only; **required** in production |
| `CORS_ALLOWED_ORIGINS` | empty (same-origin) | |
| `BREAKER_*`, `RETRY_*` | see §10 | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty (disabled) | |
| `LOG_LEVEL` | `info` | |

---

## 20. Open Questions for Review

1. Default meeting TTL 24h — acceptable, or shorter (e.g. 4h) to limit abuse?
2. Should hosts be able to *ban* by IP/64 after removal (stronger, but
   collateral damage behind NAT)? Current answer: no.
3. CAPTCHA provider, if abuse data ever justifies one (deferred, A8).
4. Domain/TLS strategy for production LiveKit (separate subdomain recommended).

---

## 21. Implementation Notes (drift from the original plan)

- **Audit writes:** the state change and its audit row are two statements
  in one transaction, not a single CTE. The effect is the same.
- **Extra modules:** `admission` (SSE hub) and `maintenance` (worker loop)
  are separate modules. `participant` is split into `Service`
  (membership), `Seats` (leases and tokens) and `Presence` (webhooks).
- **Schema additions:** `participants.status_at` (how fresh a waiting
  entry is), `participants.origin_hash`, `meetings.room_created_at` and
  `meetings.sweep_after`, plus partial indexes for the workers.
  `moderation_events.actor_id` is nullable (NULL means a system action).
- **Admitted cap:** at most 4× capacity, counting only guests active in
  the last 30 minutes.
- **LiveKit client:** the API uses the `livekit/protocol` Twirp client
  directly instead of `server-sdk-go`, which would pull in a WebRTC client
  stack.
- **Migrations:** run by the API image (`api migrate up`) in the one-shot
  `migrate` service.
- **Port names in code:** `Participants`, `Room`, `TokenSigner`,
  `SeatStore` and similar. See each module's `service.go`.
- **ADRs:** summarised in §14. The review history is in
  `docs/review-log.md`.
