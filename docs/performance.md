# Performance

- **Media path:** browsers send media directly to LiveKit (SFU), so the API
  carries only small JSON requests. Defaults:
  - simulcast (180p, 360p and 720p)
  - adaptive stream and dynacast
  - Opus DTX and RED
  - screen share at 1080p, 15 fps
- **API hot paths:**
  - Resolving a session is one indexed query that joins the participant and
    the meeting.
  - A token request runs about 6 indexed queries under a single meeting-row
    lock, and calls LiveKit only when the room is not already in use.
- **Database:**
  - pgx pool (20 by default), with a 5s statement timeout and a 10s idle
    transaction timeout.
  - Every index matches a query pattern (PLAN §6), and partial indexes cover
    the worker scans.
  - No transaction is held open across a network call.
- **Redis:** per-replica fallback if Redis is unreachable:
  - Each rate-limit decision gets a 150 ms budget, and Redis is skipped for
    1s after an error.
  - One `PSUBSCRIBE` connection per replica serves all SSE streams.
  - Chat history is capped at 100 messages and expires with the meeting.
- **Concurrency:**
  - Goroutines are bounded: the HTTP server, one hub and one maintenance
    loop per replica, and LiveKit fan-out limited to 8.
  - Everything is joined with `errgroup` on shutdown.
  - SSE streams have heartbeats, a lifetime cap and a shutdown signal.
- **Resilience:**
  - Three LiveKit circuit breakers (room, admin, data), so chat or token
    traffic cannot disable moderation.
  - Idempotent calls are retried up to 3 times with jittered backoff.
  - Side effects that run after a commit cannot be cancelled by the client.
- **Frontend:** the meeting route and the LiveKit SDK are lazy-loaded. The
  landing bundle is about 118 KB gzipped. Static assets are cached as
  immutable.
- **Load testing:** not done yet. Suggested next step: a k6 script for
  create, join and token requests, plus LiveKit's `lk load-test` for media.
