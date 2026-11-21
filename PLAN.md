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
| A3 | Removed participants rejoining | Anonymous users can always obtain a new session. Removal kills the current session; the host can **lock** or enable the **waiting room** to stop re-entry. Documented limitation. |
| A4 | Chat transport | Messages go through the backend (rate-limited, validated) and are fanned out via LiveKit server `SendData`. Clients get **no** `canPublishData` grant, so chat cannot be spoofed or flood the room. |
| A5 | Chat history | Last 100 messages kept in Redis (TTL = meeting expiry) so late joiners see context. Not persisted to Postgres. |
| A6 | Muting | Host can **mute** (server-side `MutePublishedTrack`). Host can **never unmute** others (privacy). |
| A7 | Participant limits | Global LiveKit room limit + backend admission check (default 50/meeting). |
| A8 | CAPTCHA | Interface + optional Cloudflare Turnstile verifier, **disabled by default**; only triggered when an IP crosses a soft risk threshold. |
| A9 | Host leaving | Meeting continues; host may rejoin using the same browser (cookie). No host = no moderation until rejoin. |
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
| `access` (Permissions) | Pure policy: `Can(role, action)`; session → principal resolution middleware | — | `SessionResolver` |
| `moderation` | Remove, mute, admit/deny, end — orchestrates access check + LiveKit + audit | `moderation_events` | `RoomModerator`, `AuditStore` |
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

### Domain events (in-process observer)

`ParticipantWaiting`, `ParticipantAdmitted`, `ParticipantDenied`,
`ParticipantRemoved`, `MeetingEnded`. Published to a small in-process
dispatcher; subscribers publish to Redis pub/sub (cross-instance SSE) and
update metrics. No message broker in MVP.

---

## 5. Identifiers, Credentials and Tokens

| Item | Generation | Storage | Lifetime |
| --- | --- | --- | --- |
| Meeting code | `crypto/rand`, 12 chars from a 32-char unambiguous alphabet (60 bits), shown as `xxxx-xxxx-xxxx` | `meetings.code` (unique) | Meeting lifetime |
| Meeting UUID | `uuid.New()` (v4) | `meetings.id`; LiveKit room name | Meeting lifetime |
| Session token | 32 bytes `crypto/rand`, base64url | **SHA-256 hash** in `participants.session_hash`; raw only in cookie | Meeting lifetime |
| Meeting password | user-chosen, 4–64 chars | **argon2id** hash (`x/crypto`) | Meeting lifetime |
| LiveKit JWT | Signed server-side with API secret | Never stored | **5 min** (LiveKit refreshes automatically while connected) |

Anti-enumeration: 60-bit codes + per-IP lookup limits (30/min) make guessing
infeasible (with 10⁶ live meetings, P(hit) ≈ 10⁻¹² per guess). Unknown,
expired and ended meetings all return the **same** `404`.

Session cookie:

```text
Set-Cookie: lamma_sid=<token>; Path=/api/v1/meetings/<code>;
            HttpOnly; Secure; SameSite=Strict; Max-Age=<until expiry>
```

- Path scoping lets one browser hold independent sessions in many meetings.
- `SameSite=Strict` + `Origin` check middleware on unsafe methods = CSRF defense.
- SHA-256 (not argon2) for session hashes is correct: tokens are 256-bit random.

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
  code             text NOT NULL UNIQUE,
  title            text CHECK (char_length(title) <= 120),
  password_hash    text,
  waiting_room     boolean NOT NULL DEFAULT false,
  locked           boolean NOT NULL DEFAULT false,
  status           meeting_status NOT NULL DEFAULT 'active',
  max_participants int NOT NULL CHECK (max_participants BETWEEN 2 AND 500),
  created_at       timestamptz NOT NULL DEFAULT now(),
  expires_at       timestamptz NOT NULL,
  ended_at         timestamptz
);
```

```sql
CREATE INDEX meetings_expiry_active ON meetings (expires_at) WHERE status = 'active';
CREATE INDEX meetings_ended_at ON meetings (ended_at) WHERE status = 'ended';
```

```sql
CREATE TYPE participant_role AS ENUM ('host', 'guest');
CREATE TYPE participant_status AS ENUM ('waiting','admitted','denied','removed','left');
CREATE TABLE participants (
  id            uuid PRIMARY KEY,                     -- uuidv7
  meeting_id    uuid NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
  display_name  text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 40),
  role          participant_role NOT NULL,
  status        participant_status NOT NULL,
  in_room       boolean NOT NULL DEFAULT false,
  session_hash  bytea NOT NULL UNIQUE,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX participants_meeting_status ON participants (meeting_id, status);
```

```sql
CREATE TABLE moderation_events (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  meeting_id     uuid NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
  actor_id       uuid NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
  action         text NOT NULL,   -- remove|mute|admit|deny|lock|unlock|end
  target_id      uuid REFERENCES participants(id) ON DELETE CASCADE,
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX moderation_events_meeting ON moderation_events (meeting_id, created_at);
```

### Query patterns → indexes

| Hot query | Index used |
| --- | --- |
| Resolve session (every authenticated request) | `participants.session_hash` unique |
| Lookup meeting by code (join, public info) | `meetings.code` unique |
| Count admitted participants at join (capacity) | `participants_meeting_status` |
| List waiting participants (host) | `participants_meeting_status` |
| Expire meetings batch | `meetings_expiry_active` (partial) |
| Purge ended meetings after retention | `meetings_ended_at` (partial) |

### Transactions and concurrency

- **Admission at capacity**: `SELECT … FROM meetings WHERE id=$1 FOR UPDATE`,
  count admitted, insert/update participant — one transaction, so concurrent
  joins cannot exceed `max_participants`.
- **Session resolve** joins `participants` ⋈ `meetings` in **one** query
  (no N+1): returns principal + meeting status/lock in a single round trip.
- **Expiry job**: `UPDATE meetings SET status='ended' … WHERE id IN (SELECT id …
  WHERE status='active' AND expires_at < now() ORDER BY expires_at LIMIT 100
  FOR UPDATE SKIP LOCKED) RETURNING id` — safe with N replicas, no Redis lock.
- **Retention**: ended meetings hard-deleted after `MEETING_RETENTION` (default 7d), cascading.

All SQL lives in `backend/db/queries/*.sql`, compiled by **sqlc** to type-safe
pgx code. Migrations via **golang-migrate** (`backend/db/migrations`), run by
a one-shot `migrate` container before the API starts (never by API replicas
racing each other).

---

## 7. Redis Usage (never the source of truth)

| Key pattern | Purpose | TTL |
| --- | --- | --- |
| `rl:{policy}:{scope}` | GCRA rate-limit state (`go-redis/redis_rate`) | Auto (window) |
| `chat:{meeting_uuid}` | Capped list (`LPUSH` + `LTRIM 0 99`) of recent chat | Until `expires_at` |
| `admission:{meeting_uuid}` | Pub/sub channel: admit/deny events to SSE streams | n/a |
| `pwfail:{meeting_uuid}` | Failed password counter (brute-force lockout) | 15 min |

Redis outage policy:

- Rate limiting: **fail-closed** for meeting creation and password attempts
  (abuse-sensitive); **fail-open** for everything else (availability). Each
  failure increments `redis_errors_total` and is logged.
- Chat history unavailable → chat send still works (broadcast is LiveKit).
- SSE admission falls back to client retry; readiness turns **not ready**.

---

## 8. API Contract (v1)

Base path `/api/v1`. JSON only. Errors use **RFC 9457** `application/problem+json`
with a stable `code` field, never internal details. Full spec: `api/openapi.yaml`
(Phase 3). Auth column: `—` public, `S` any valid session, `H` host session.

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| POST | `/meetings` | — | Create meeting; caller becomes host (sets cookie) |
| GET | `/meetings/{code}` | — | Public info: `title, requiresPassword, waitingRoom, locked` |
| POST | `/meetings/{code}/join` | — | Join with `displayName`, `password?` → `admitted` or `waiting` (sets cookie) |
| GET | `/meetings/{code}/me` | S | Current principal (`participantId, role, status`) |
| GET | `/meetings/{code}/admission/events` | S | **SSE** stream: `admitted` / `denied` while waiting |
| POST | `/meetings/{code}/token` | S (admitted) | Issue LiveKit JWT `{token, url, expiresAt}` |
| POST | `/meetings/{code}/leave` | S | Mark left, clear cookie |
| GET | `/meetings/{code}/chat` | S (admitted) | Recent chat messages |
| POST | `/meetings/{code}/chat` | S (admitted) | Send chat `{text ≤ 1000}` |
| GET | `/meetings/{code}/participants?status=waiting` | H | Waiting list |
| POST | `/meetings/{code}/participants/{id}/admit` | H | Admit from waiting room |
| POST | `/meetings/{code}/participants/{id}/deny` | H | Deny |
| POST | `/meetings/{code}/participants/{id}/remove` | H | Kick (LiveKit `RemoveParticipant` + revoke session) |
| POST | `/meetings/{code}/participants/{id}/mute` | H | `{source: microphone|camera|screen_share}` |
| PATCH | `/meetings/{code}/settings` | H | `{locked?, waitingRoom?}` |
| POST | `/meetings/{code}/end` | H | End for all (`DeleteRoom`) |
| POST | `/internal/livekit/webhook` | LiveKit signature | Join/leave/room events (not routed by Nginx) |
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

Join outcomes: `200 admitted`, `202 waiting`, `401 password_required|invalid_password`,
`403 meeting_locked`, `409 meeting_full`, `404 not_found`, `429 rate_limited`
(with `Retry-After`).

Authorization guarantees:

- Role comes **only** from the session row. Request bodies never carry roles.
- Every host route runs `RequireRole(host)` middleware **and** the service
  re-checks `access.Can(principal, action)` — defense in depth.
- Target participant must belong to the principal's meeting (IDOR check in SQL:
  `WHERE id=$1 AND meeting_id=$2`).
- `removed`/`denied`/`left` sessions cannot obtain tokens; ended meetings
  reject everything except `GET /me`.

---

## 9. Key Flows

**Create** → validate → generate code (retry on unique collision, max 3) →
insert meeting + host participant in one tx → set cookie → 201. No LiveKit call.

**Join** → rate limit (IP + meeting) → load meeting → reject if ended/locked →
verify password (argon2id; failures counted in `pwfail`) → tx: capacity check,
insert participant `waiting|admitted` → set cookie → if waiting, emit
`ParticipantWaiting` (host notified via LiveKit `SendData` to host identity).

**Waiting room** → client opens SSE → server subscribes to
`admission:{meeting}` → host admits/denies → event published → stream emits →
client calls `/token`. SSE has heartbeat (25s), max lifetime (10 min), and the
subscription is closed on `ctx.Done()` (no goroutine leak).

**Token** → resolve principal → must be `admitted` and meeting `active` →
sign JWT (local, no network) → return.

**Remove** → host check → tx: set target `removed`, rotate nothing (session hash
now dead), audit row → LiveKit `RemoveParticipant` (breaker + retry; `not_found`
= success) → 204. If LiveKit is down, DB state still blocks rejoin; response
`202` with `livekitPending=true` and a reconciliation retry is scheduled in the
bounded worker pool.

**End** → host check → `status=ended` → LiveKit `DeleteRoom` → clear chat key.

**Webhooks** → verify signature (`webhook.ReceiveWebhookEvent`) →
`participant_joined/left` → update `in_room` → metrics. Idempotent.

---

## 10. Resilience

### Rate limits (Redis GCRA, all env-configurable)

| Policy | Key | Default |
| --- | --- | --- |
| General API | IP | 120/min |
| Meeting create | IP | 10/hour |
| Meeting lookup | IP | 30/min |
| Join | IP + meeting | 10/min, 60/min per meeting |
| Password attempts | IP + meeting | 5 per 15 min; meeting-wide 50 → 15-min lockout |
| Token issue | session | 30/min |
| Chat | session | 20 per 10s, 1000 chars |
| Moderation | session | 60/min |

- Client IP: `X-Forwarded-For` honored **only** from `TRUSTED_PROXY_CIDRS`
  (rightmost untrusted hop); otherwise `RemoteAddr`. Prevents header spoofing bypass.
- IPv6 clients limited by **/64** prefix to stop address rotation.
- 429 responses include `Retry-After` and increment
  `ratelimit_rejections_total{policy}`.

### Circuit breaker — `sony/gobreaker/v2`

Wraps **only** LiveKit server API calls (and future external webhooks). Not
DB/Redis (pools + timeouts suffice; a breaker there hides real outages).

| Setting | Default |
| --- | --- |
| Trip | ≥ 5 consecutive failures, or ≥ 50% failures over ≥ 20 requests in 30s |
| Open timeout (→ half-open) | 30s |
| Half-open max probes | 3 |
| Per-call timeout | 3s (`context.WithTimeout`) |

State exported as `circuit_breaker_state{name}` (0 closed, 1 half-open, 2 open).
Open → API returns `503` + `Retry-After` for moderation; joins/tokens unaffected
(token signing is local).

### Retries — `cenkalti/backoff/v5`

- Only **idempotent** ops: `RemoveParticipant`, `MutePublishedTrack`,
  `DeleteRoom`, `GetParticipant`. `SendData` is **not** retried.
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

### Background work

- **Expiry worker**: one goroutine per replica, ticker (30s), `SKIP LOCKED`
  batches; LiveKit `DeleteRoom` fan-out through a **bounded** semaphore (size 8).
- **Reconciler**: bounded queue (cap 1024, drop-with-metric on overflow) for
  LiveKit ops that failed after DB commit.
- All workers take a root `context` and are joined with `errgroup` on shutdown.

### Graceful shutdown

`signal.NotifyContext(SIGINT, SIGTERM)` → readiness flips to 503 → `server.Shutdown`
(deadline 20s; SSE streams get a close event) → cancel worker ctx → wait
errgroup → flush OTel → close Redis → close pgx pool. Compose
`stop_grace_period: 30s`.

---

## 11. Security Design

- **Secrets**: `LIVEKIT_API_SECRET`, DB/Redis passwords only via env/secret files.
  Never in images, logs, responses or the frontend bundle. Config struct has a
  redacting `LogValue()`.
- **Authorization**: §8 guarantees. Role never accepted from client.
- **Input validation**: hand-written validators per DTO (lengths, enums, Unicode
  normalization NFC, control-char stripping for display names/chat). Unknown
  JSON fields rejected (`DisallowUnknownFields`).
- **SQL injection**: impossible by construction — sqlc parameterized queries only.
- **CSRF**: `SameSite=Strict` cookies + `Origin`/`Sec-Fetch-Site` check on
  unsafe methods + JSON-only content type.
- **CORS**: same-origin by default (SPA served by same Nginx). Explicit
  allowlist via `CORS_ALLOWED_ORIGINS`; no wildcard with credentials.
- **Security headers** (Nginx + API): CSP (`default-src 'self'; connect-src
  'self' wss://<livekit>; frame-ancestors 'none'`), HSTS (prod),
  `X-Content-Type-Options: nosniff`, **`Referrer-Policy: no-referrer`** (meeting
  URLs are capabilities), `Permissions-Policy: camera=(self), microphone=(self),
  display-capture=(self)`.
- **Brute force**: password attempt limits per IP and per meeting, plus a
  meeting-wide lockout. Constant-time comparison is inherent to argon2id
  verification. (No dummy-hash timing equalisation: whether a meeting has a
  password is already public via `GET /meetings/{code}`.)
- **Enumeration**: 60-bit codes, uniform 404, lookup rate limit.
- **Abuse**: creation limits, global per-IP limits, max participants,
  max meeting duration, optional Turnstile challenge (A8).
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
  pings Postgres + Redis (500ms timeout each, results cached 2s to avoid probe
  storms); LiveKit reachability reported in body but does **not** fail readiness
  (joins still work; moderation degrades behind the breaker).

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
├── db/sqlc/                 # generated (committed, CI checks drift)
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

Jobs (actions pinned by commit SHA; Go/Node versions pinned):

1. **format** — `gofmt -l`, `prettier --check`.
2. **go-lint** — `golangci-lint` with `funlen` (**statements: 15**), `gocognit`,
   `nestif`, `errcheck`, `gosec`, `bodyclose`, `contextcheck`, `noctx`.
3. **backend-test** — `go test -race -cover ./...`; `sqlc diff` (generated code drift).
4. **backend-integration** — `go test -race -tags=integration ./...` (testcontainers).
5. **frontend** — `eslint`, `tsc --noEmit`, `vitest run`.
6. **docker** — `docker buildx build` for api and web (no push on PRs).
7. **security** — `govulncheck`, `npm audit --audit-level=high`, Trivy (fs +
   images), `gitleaks`.

The 15-line rule is enforced mechanically by `funlen` (statements) and by the
`backend-architect` agent for judgment cases.

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
| Rate limiting across instances | Integration test with 2 limiter clients on one Redis |
| Graceful handling of dependency failure | Breaker tests; LiveKit-down integration test |
| Starts via Docker Compose | CI docker job + compose smoke test |
| Automated tests pass | CI green |
| No critical security issues | `security-performance-reviewer` sign-off |

---

## 19. Environment Variables (reference; full list in `.env.example`)

| Variable | Default | Notes |
| --- | --- | --- |
| `APP_ENV` | `development` | `production` enables HSTS, Secure-only cookies |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Used to build meeting URLs |
| `HTTP_ADDR` / `METRICS_ADDR` | `:8000` / `:9090` | Metrics port is internal-only |
| `DATABASE_URL` | — | Required |
| `DB_MAX_CONNS` | `20` | pgx pool |
| `REDIS_URL` | — | Required |
| `LIVEKIT_URL` / `LIVEKIT_PUBLIC_URL` | — | Internal API URL / browser `wss://` URL |
| `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET` | — | Secret: server only |
| `LIVEKIT_TOKEN_TTL` | `5m` | |
| `MEETING_DEFAULT_TTL` / `MEETING_MAX_TTL` | `24h` / `168h` | |
| `MEETING_MAX_PARTICIPANTS` | `50` | |
| `MEETING_RETENTION` | `168h` | Hard-delete ended meetings after |
| `RATE_LIMIT_*` | see §10 | One var per policy, e.g. `RATE_LIMIT_CREATE=10/h` |
| `TRUSTED_PROXY_CIDRS` | `172.16.0.0/12` | Docker network in dev |
| `CORS_ALLOWED_ORIGINS` | empty (same-origin) | |
| `BREAKER_*`, `RETRY_*` | see §10 | |
| `TURNSTILE_SECRET` | empty (disabled) | Optional CAPTCHA |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty (disabled) | |
| `LOG_LEVEL` | `info` | |

---

## 20. Open Questions for Review

1. Default meeting TTL 24h — acceptable, or shorter (e.g. 4h) to limit abuse?
2. Should hosts be able to *ban* by IP/64 after removal (stronger, but
   collateral damage behind NAT)? Current answer: no.
3. Turnstile vs hCaptcha if CAPTCHA gets enabled.
4. Domain/TLS strategy for production LiveKit (separate subdomain recommended).
