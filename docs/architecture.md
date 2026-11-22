# Architecture

Lamma is a **modular monolith**: one Go binary, one database, with business
modules kept separate by package boundaries and narrow interfaces. Media
never passes through the API: browsers send audio and video to LiveKit, and
the API only issues short-lived LiveKit tokens and calls LiveKit's server
API for moderation. The full rationale is in [PLAN.md](../PLAN.md).

```text
Browser ──HTTPS──► Nginx ──/api──► Go API (stateless, N replicas) ──► Postgres
   │                                │      ▲                        └► Redis
   └──WSS + UDP/TCP/TURN──► LiveKit ◄┘      └── signed webhooks
```

## Modules (`backend/internal`)

| Module | Owns |
| --- | --- |
| `meeting` | Creating meetings, public lookup, settings, ending, and LiveKit room bookkeeping |
| `participant` | Joining and resuming, the waiting room, status changes plus audit (`Service`), seat leases and tokens (`Seats`), webhook presence (`Presence`) |
| `access` | Session tokens and cookies, the session-to-principal read model, the permission policy `Can(role, action)` |
| `moderation` | Host actions. Owns no tables: it authorizes, calls the module that owns the data, then calls LiveKit after the commit |
| `chat` | Validates and rate-limits messages, broadcasts through LiveKit, keeps Redis history |
| `admission` | Waiting-room decisions over SSE, through one Redis `PSUBSCRIBE` per replica |
| `realtime` | LiveKit adapter: token signing, the Twirp room API behind circuit breakers, webhooks |
| `maintenance` | Expiry, room and intruder reconciliation, the anti-entropy sweep, purge, gauges |
| `platform/*` | Config, logging, Postgres, Redis, HTTP helpers, rate limiting, resilience, metrics, tracing |

Dependencies point inward. Service files contain no HTTP, pgx, Redis or
LiveKit types; those live in `http.go`, `postgres.go`, `redis.go` and
`realtime`. Each module declares the interfaces it consumes. Everything is
wired once in `internal/app`. sqlc generates one query package per module, so
no module can call another module's queries.

## Key flows

- **Create:** in one transaction, insert the meeting and its host
  participant, then set an HttpOnly session cookie scoped to
  `/api/v1/meetings/{code}`. No LiveKit call is made.
- **Join:** under a lock on the meeting row, check the lock state, the
  password (its rate limit is spent before argon2 runs) and the
  waiting-room caps, then insert the participant as `admitted` or
  `waiting`. With a valid session cookie, joining again resumes the same
  participant.
- **Token:** ensure the LiveKit room exists, then claim a seat lease in a
  transaction (capacity and per-origin caps), then sign a 2-minute JWT
  locally.
- **Moderation:** authorize the host, have the owning module change state
  and write the audit row atomically, then make the LiveKit call after the
  commit. If that call fails, the response is `202 livekitPending` and the
  reconciler finishes the job.
- **Webhooks:** verify the signature and body hash, record presence, and
  evict anyone connected who is not an admitted participant of that
  meeting.
- **Maintenance:** every 30 seconds, using `SKIP LOCKED` claims: expire
  meetings, close rooms of ended meetings, evict intruders, sweep LiveKit
  against the database, purge old meetings and update gauges.

## Frontend (`frontend/src`)

- `features/*`: one folder per screen or feature (home, meeting, prejoin,
  waiting, room, chat, participants, moderation, reactions, whiteboard,
  recording, effects).
- Stage transitions all live in `features/meeting/session.ts`. Components
  call those functions; they never write the stage directly.
- `stores/meeting.ts` (Zustand) holds meeting, session and device
  preferences only. Media state stays in LiveKit, and roles shown in the UI
  are for display only.
- `stores/collab.ts` holds in-room collaboration state: the host's
  announced state (recording, whiteboard mode, drawers, draw requests) and
  the whiteboard strokes.
- **Reactions, the whiteboard and the recording indicator** run peer to peer over LiveKit data
  messages (`features/whiteboard/protocol.ts` defines and validates them).
  The host's browser is the authority: it announces state, answers late
  joiners with a snapshot, and approves draw requests. See
  [security.md](security.md#peer-to-peer-collaboration).
- **Layout:** below 768px the control bar keeps mic, camera, reactions,
  chat and leave, and moves the rest into a "More" sheet. The whiteboard or
  a screen share takes focus, with the cameras in a strip beside it
  (desktop) or below it (phone). Recording media never leaves the host's
  browser; only the on/off flag is shared.
- **Camera effects** (`features/effects`) are a LiveKit `TrackProcessor`.
  Each camera frame is drawn on a canvas: the background is replaced using
  MediaPipe's selfie segmenter, and face masks are drawn from the face
  landmarker's points. LiveKit publishes `canvas.captureStream()` instead of
  the camera. MediaPipe runs in a module Web Worker
  (`vision.worker.ts`), one frame at a time. The page only composites, and
  skips frames so effects use at most a third of the main thread. The
  processor is attached only while an effect is on. If an
  effect is chosen before joining, it goes into the capture options so the
  first published frame is already processed. MediaPipe lives only in the
  worker bundle, which loads on first use. Its wasm and models are served from `/vision/` on
  our own origin. `scripts/vision-assets.mjs` copies and downloads them at
  build time and verifies their checksums.
- `shared/api`: the typed client for our REST API (`/api/v1`), described in
  [api/openapi.yaml](../api/openapi.yaml).

## Decisions

The ADR summary table is in [PLAN.md §14](../PLAN.md). ADR-001 to ADR-015
cover: the monolith, LiveKit, cookie sessions, chat through the API, SSE
admission, GCRA rate limiting, scoped circuit breakers, `SKIP LOCKED`
workers, internal UUIDs versus public codes, stdlib config, one-shot
migrations, a table-less moderation module, seat leases, the webhook guard
with reconciliation, and the per-replica SSE hub. Review findings and their
resolutions are in [review-log.md](review-log.md).
