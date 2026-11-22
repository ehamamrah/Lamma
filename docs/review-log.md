# Review Log

Each phase is reviewed by `backend-architect` (BA) and
`security-performance-reviewer` (SP). Critical/High findings must be resolved
before the next phase. Medium findings are fixed or accepted here with a reason.

## Phase 1 — Architecture (PLAN.md)

### backend-architect — verdict: BLOCK → resolved

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| BA-1 | High | Ghost "admitted" participants count toward capacity forever | Capacity counts `admitted AND (in_room OR admitted_at > now()-2m)`; `participant_left` webhook clears `in_room` (§6) |
| BA-2 | High | In-memory reconciler queue loses LiveKit ops | Queue removed. DB-driven reconciliation: `meetings.room_closed_at`, removed-but-in-room sweep (§10) |
| BA-3 | High | `moderation` writing other modules' tables | `moderation` is a table-less orchestrator; owning modules write their rows + audit row in their own tx (ADR-012) |
| BA-4 | High | Pub/sub admit lost before SSE subscribes | Publish after commit; SSE subscribes first, then reads DB status and emits immediately if decided (§9) |
| BA-5 | High | `/leave` clears host cookie, contradicting A9 | Sessions are never cleared on leave; `/join` with a valid session cookie is idempotent and resumes the same principal (§8, A9) |
| BA-6 | Med | Admit bypasses capacity tx | Moot: capacity is enforced at token time (seat claim tx), so admit needs no check |
| BA-7 | Med | One shared sqlc package erodes boundaries | sqlc generates per-module packages; session resolve is an explicit `access` read model |
| BA-8 | Med | Expiry lag allows late tokens | All guard queries include `expires_at > now()` |
| BA-9 | Med | SSE keeps running through `Shutdown` | `BaseContext` cancelled before `Shutdown`; 5s drain after readiness flips |
| BA-10 | Med | One Redis connection per SSE stream | One `PSUBSCRIBE` per replica + in-process hub keyed by participant |
| BA-11 | Med | Out-of-order webhooks corrupt `in_room` | `in_room_at` stores event time; stale events ignored |
| BA-12 | Med | Mute needs track SID | Adapter `MuteSource(room, identity, source)` = lookup + mute, retried as one unit |
| BA-13 | Med | Cookie path depends on code casing | Canonical code form lowercase-with-dashes; the SPA normalises and redirects, the API accepts only canonical codes |
| BA-14 | Low | Domain-event dispatcher is indirection | Removed; direct `AdmissionNotifier` call after commit |
| BA-15 | Low | `meetings_active` gauge per replica | Documented: aggregate with `max()` |
| BA-16 | Low | Breaker counts `not_found` as failure | Two breakers (`livekit_admin`, `livekit_data`, see SP-5); `IsSuccessful` treats not-found and client errors as success |
| BA-17 | Info | Turnstile speculative | Deferred; hook point only |

### security-performance-reviewer — verdict: FAIL → resolved

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| SP-1 | Critical | Unbounded argon2 concurrency → memory exhaustion | Weighted semaphore gate (`503 server_busy`), m=19 MiB/t=2/p=1, limits consumed before verify, 8-char minimum (§11) |
| SP-2 | High | Removed/ended users re-enter LiveKit via refreshed tokens / auto-create | `auto_create: false`, `CreateRoom` at token time, `participant_joined` DB guard, DB-driven reconciler (ADR-014) |
| SP-3 | High | Capacity/waiting-room flooding | Seat leases (ADR-013), waiting cap 50 + 10-min staleness, waiting list `LIMIT 100`, sessions exempt from per-meeting join limit |
| SP-4 | High | SSE connection exhaustion + missed events | Per-replica `PSUBSCRIBE` hub, 3/IP + 2000/replica caps, subscribe-then-check-DB, per-participant channel, Nginx buffering off |
| SP-5 | High | Guests can trip the moderation breaker | Separate `livekit_admin` / `livekit_data` breakers, `IsSuccessful` excludes cancel/not-found/4xx, `WithoutCancel` post-commit, meeting-wide chat limit |
| SP-6 | High | XFF trust range too broad | Nginx fixed IP `172.30.0.10/32`; API port unpublished; required in prod |
| SP-7 | Med | Meeting-wide lockout weaponisable; weak passwords | No hard lockout; per-meeting 100/h; 8-char minimum |
| SP-8 | Med | Readiness on Redis drains all replicas | Readiness = Postgres only; per-replica coarse fallback limiter for fail-open policies |
| SP-9 | Med | Code normalisation | Canonical lowercase dashed form; SPA normalises; limit keys use UUID |
| SP-10 | Med | Out-of-order webhooks | `in_room_at` ordering |
| SP-11 | Med | Dev LiveKit keys in prod | Production startup guards (§11) |
| SP-12 | Low | CSP missing `https://<livekit>` | Added; Turnstile deferred |
| SP-13 | Low | Cookie flags inconsistent | `COOKIE_SECURE`, `__Secure-` prefix in prod, documented dev behaviour |
| SP-14 | Low | Bidi/impersonation in names | Strip Cf; role badge separate from name |
| SP-15 | Low | SSE holds shutdown | `RegisterOnShutdown` cancels hub |
| SP-U1 | Unverified | TURN relay to internal addresses | Verify in Phase 2; firewall private ranges in prod |

### Re-verification round 1

backend-architect: 14/17 resolved; new findings resolved as follows.

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| N1 | High | Same-second join/left drops `left`, seat stuck | `livekit_sid` match + `>=` comparison |
| N2 | High | Guarded joins never recorded → reconciler blind | Webhook always records `in_room`; sweep `in_room AND status <> 'admitted'` |
| N3 | Med | Reconciler claim/backoff undefined; not_found loops | `reconcile_after` claim via `UPDATE … RETURNING`; not_found clears |
| N4 | Med | Cancelling `BaseContext` aborts in-flight tx | Cancel only the SSE hub |
| N5 | Med | Host locked out of full meeting | Host exempt from seat count; LiveKit cap = max+1 |
| N6 | Med | Lease shorter than JWT | Token TTL = lease = 2 min |
| N7 | Low | `room_started` race after End | Clear `room_closed_at` so reconciler retries |
| D1/D2 | Low | Doc drift | Fixed |

security-performance-reviewer: SP-1, 4, 5, 8–12, 14, 15 resolved; remaining/new resolved as follows.

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| N1 | High | `/token` → `CreateRoom` lets guests trip the admin breaker | `room_created_at` (CreateRoom once per room), dedicated `livekit_room` breaker, token limit 6/min |
| N2 | High | Waiting room can be held full | 3 waiters per origin per meeting, no silent clock refresh, host deny-all-waiting |
| SP-7 | Med | Per-meeting password budget = lockout | Removed; per-IP only |
| SP-3 | Med | Lease renewal squatting | Max 3 leases without `participant_joined` |
| SP-2 | Med | Lost webhook leaves intruder | 5-min `ListParticipants` anti-entropy sweep |
| N3 | Med | LiveKit call inside seat tx | Tx commits before any LiveKit call |
| SP-6 | Med | LB in front of Nginx collapses IPs | Nginx `real_ip` config for prod |
| L1 | Low | Stripping all Cf breaks ZWJ/ZWNJ | Strip only bidi controls |
| L2 | Low | Nginx worker_connections | 4096 |
| L3 | Low | Cookie example drift | Fixed |

### Re-verification round 2 (final)

- security-performance-reviewer: **PASS WITH CHANGES**, no Critical/High.
- backend-architect: one High (H1) — a lost `room_finished` webhook stranded a meeting.

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| H1 | High | Lost `room_finished` → `CreateRoom` skipped forever | Skip `CreateRoom` only when someone is `in_room`; sweep clears `room_created_at` on `not_found` |
| S-M2 | Med | One IP fills a meeting by minting sessions | Max 3 leased-but-unconnected seats per origin per meeting |
| Low | Low | Stale 60s/refresh text | Fixed |

**Phase 1 gate: passed.** No unresolved Critical/High findings. Open item carried to
Phase 2: SP-U1 (verify LiveKit TURN cannot relay to internal addresses).

## Phase 2–3 — Infrastructure & backend foundation

- backend-architect: **APPROVE WITH CHANGES** (no Critical/High).
- security-performance-reviewer: **PASS WITH CHANGES** (no Critical/High).

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| BA-1 | Med | gofmt drift | `gofmt -w`; CI enforces |
| BA-2 | Med | `InTx` leaks tx on panic | Deferred rollback + re-panic |
| BA-3 | Med | Implicit tx contract | Documented rules on `TxManager` (Conn-only, no network calls, cross-module only via services) |
| BA-4 | Med | Breaker zero-value settings unsafe | `withDefaults` (3s timeout, threshold 5, cancel ≠ failure) |
| BA-5 | Med | Services would import `ratelimit` (HTTP deps) | `ratelimit.Check` func port bound in app wiring via `Limiter.For` |
| BA-6 | Low | Unused `apperr.Accepted` | Removed |
| BA-7 | Low | Weak audit/meeting integrity | `action` CHECK, `ended_at`↔`status` CHECK, `ON DELETE SET NULL`, app role REVOKE UPDATE/DELETE on audit |
| BA-8/9 | Low | Receiver-less method; fixed healthcheck port | Plain literal; port documented as fixed |
| SP-1 | Med | TURN relay deny list implicit | Explicit RFC1918/loopback/link-local/CGNAT/ULA deny list |
| SP-2 | Med | `APP_ENV` defaulted to development | Defaults to `production` (guards on) |
| SP-3 | Med | Degraded limiter shared across clients | Per-client fallback, bounded to 10k keys |
| SP-4 | Med | Blackholed Redis adds latency | 150ms budget per decision + 1s skip window after errors |
| SP-5 | Med | Unindexed audit FKs | Partial indexes on `actor_id`, `target_id` |
| SP-6 | Low | Prod guards incomplete | `wss://`/`https://` enforced for public URLs and CORS origins |
| SP-7 | Low | Trailing JSON accepted | Second `Decode` must hit EOF |
| SP-8 | Low | `Vary: Origin` conditional | Always set |
| SP-9 | Low | Redis password in argv | Written to tmpfs config file |
| SP-10 | Low | Data containers writable; floating tags | `read_only` + tmpfs for Postgres/Redis; all base images later pinned by digest (Phase 5) |
| SP-11 | Low | No idle-in-tx timeout | `idle_in_transaction_session_timeout=10s` |
| SP-12 | Low | Raw IPs in Redis keys | Keys are HMAC'd |
| Handoffs | — | Recover after headers; detached readiness probes; cancel ≠ Redis error | All fixed |
| SP-13 | Info | SSE write deadline / shutdown hook | Implemented in Phase 4 (admission hub) |

## Phase 4 — Business logic

- backend-architect: **APPROVE WITH CHANGES** (no Critical/High).
- security-performance-reviewer: **FAIL** (1 High) → fixed, re-verification requested.

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| SP4-1 | **High** | Lease counter never reset → permanent media lockout (host too); LiveKit outage consumed attempts | Sliding 10-min lease window in SQL; host exempt; room ensured *before* the seat claim; e2e regression test |
| SP4-2 | Med | "Someone is waiting" broadcast to the whole room | Sent to the host identity only, 1s budget |
| SP4-3 | Med | Unbounded admitted rows; sweep loaded all admitted IDs | Admitted guests capped at 4× capacity; sweep filters only connected identities (`= ANY`) |
| SP4-4 | Med | Meeting-wide join limit = lockout vector | Keyed per IP per meeting (5/min), session holders exempt |
| SP4-5 | Low | SSE headers not flushed | Flushed on open |
| SP4-6 | Low | Token TTL unbounded | Validated 30s–5m |
| SP4-7 | Low | Host list polling shared the moderation budget | GET excluded from the moderation limit |
| SP4-8 | Low | `MarkJoined` not scoped to room | Scoped by `meeting_id`; foreign rooms/identities evicted |
| SP4-9 | Low | ~6 queries per token request | **Accepted** for MVP (indexed, single meeting row lock) |
| SP4-10 | Info | Removed guests can rejoin with a new session | **Accepted** (A3); lock / waiting room are the controls |
| BA4-1 | Med | Modules bypassed meeting service via its store | Room bookkeeping on `meeting.Service`; store `Lookup` and glue adapter removed |
| BA4-2 | Med | Exported embedded `Deps` | Unexported `d Deps` |
| BA4-3 | Med | Participant service nearing god object | `Seats` split out with its own narrow `SeatStore` |
| BA4-4 | Med | Maintenance imported adapter / domain type in infra file | `ListIdentities` returns `exists`; `Intruder` moved to domain file |
| BA4-5 | Med | No index for `in_room` queries | `participants_in_room` partial index |
| BA4-6 | Med | Remove: read-then-write race | Single guarded `RemoveGuest` statement returning previous status |
| BA4-7/8 | Low | Transport imported adapter; mute audited before success | Source whitelist in moderation; mute then audit |
| BA4-9/10 | Low | Silent occupancy error; two-statement leave | Debug log; leave in one transaction |
| BA4-12 | Info | `admission.wait` ~20 lines | **Accepted exception**: single `select` loop; splitting harms clarity |

Re-verification: **PASS WITH CHANGES** — SP4-1 confirmed fixed live; one new Medium (admitted cap counted idle rows forever) fixed by counting only recently active admitted guests (30 min). **Phase 4 gate: passed.**

## Phase 5 — Frontend

- backend-architect: **APPROVE WITH CHANGES**; security-performance-reviewer: **PASS WITH CHANGES** (no Critical/High).

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| FA-1 | Med | Host never alerted to waiters with panel closed | Waiting list lifted to the room; badge on Host button |
| FA-2/3 | Med | Stage writes scattered; no reset/stale guard | Transitions centralised in `session.ts`; reset on unmount; stale-response guard |
| FA-4 | Med | No LiveKit `onError` | Mapped to the error stage |
| FA-5 | Low | New capture objects each render re-ran room setup | Memoised (also fixed flaky e2e) |
| FA-6..11 | Low | Shared error state, admission unmount race, type drift, selectors | Panel-local errors, active flag, contract-aligned types, `useShallow` |
| FS-1 | Med | Refused SSE left guest stuck | Status recheck + jittered reopen; server sends `retry:` and Retry-After |
| FS-2..8 | Low | Template dir, digests, CSP worker-src, duplicate API headers, debounce, prod parity | Private `/tmp/nginx`, digests pinned, `worker-src 'self'`, `proxy_hide_header`, 500ms debounce, prod `/assets` + `/metrics` |
| Bug | — | Chat panel crashed in Chrome 159 (`scrollIntoView` returns a Promise from an effect) | Block-bodied effect; caught by the browser e2e |

## Phase 6 — Final review

- security-performance-reviewer: **FAIL** (1 High) → fixed.
- backend-architect: **APPROVE WITH CHANGES** (no Critical/High).

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| SP6-1 | **High** | Client hang-ups counted as Redis failures → anyone could force degraded mode (503 on create/password) | Cancelled callers short-circuit without touching the skip window; regression test |
| SP6-2 | Low | `//nolint` comment swallowed room timeouts | Fixed; audit found the same bug dropping `ExpiresAt` on meeting insert (caught by e2e), fixed |
| SP6-3..5 | Low | Prod asset HSTS; unpinned lint/vuln tools; Gitleaks false positives | HSTS added; tools pinned; `.gitleaks.toml` allowlist (scan clean) |
| BA6-1 | Med | No cross-instance rate-limit test | `TestLimitIsSharedAcrossInstances` |
| BA6-2 | Med | Chat untested | `chat_test.go` |
| BA6-3 | Med | Invalid `expiresInMinutes` silently defaulted | Now rejected with 400 |
| BA6-4..6 | Low | OpenAPI gaps, CI no-op step, plan drift | Fixed |
| SP6-6 | Info | Author email becomes public | **Accepted**: the user asked for commits under the local git identity |

## Phase 7 — Mobile, no participant cap, reactions, whiteboard, recording

- security-performance-reviewer: **FAIL** (2 High) → fixed.
- backend-architect: **APPROVE WITH CHANGES** (no High).

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| SP7-1 | **High** | With data publishing on, a guest could spoof server chat in the window where receivers don't know the sender yet (`from` undefined) | `onServerData` accepts only packets with no participant identity (unforgeable); used for chat and waiting signals; unit test |
| SP7-2 | **High** | Every pointer move copied state and fully redrew/reallocated the canvas | Canvas resized only on resize; redraws via store subscription, at most once per frame; budgets lowered |
| SP7-3 | Med | No board rate gate; one drawer could use every stroke slot | 40 msgs/s per drawer; 2k points per stroke, 300 strokes per author, 200k points per board |
| SP7-4 | Med | `hello` floods made the host re-serialise the board | Snapshot cached by reference; 3/min per guest and 5/s overall |
| SP7-5 | Med | No participant ceiling at all | **Accepted**: product requirement (no cap). Host lock / waiting room; per-IP join limits; documented in security.md |
| SP7-6 | Med | Any guest can flood data messages | **Accepted** and documented; receive-side gates; host can remove |
| SP7-7/8 | Med/Low | Recording held in RAM unbounded; capture leaked if setup threw | 2.5 Mbps, auto-stop and save at 1.5 GB; cleanup on setup errors |
| SP7-9..12 | Low | Reaction re-render storm; rate-gate keys never pruned; stale comment | Reactions in their own store read only by the overlay, plus a room-wide gate; idle keys pruned; comment fixed |
| BA7-1 | Med | Host reload left guests with a stale board | Guests drop the board when the host leaves; snapshots merge with live strokes |
| BA7-3 | Med | Dropping a column breaks old replicas mid-deploy | Stop-migrate-start procedure in operations.md |
| BA7-4 | Med | Receive-side authorization untested | `useCollabSync.test.ts` covers host-state spoofing, unapproved strokes, guest clear, gates |
| BA7-5..7 | Med/Low | Stale capacity and data-grant wording in PLAN, docs and comments | Updated (A4, §5, §6, ADR-004/013, test plan) |
| BA7-8..12 | Low | Store/feature import cycle, empty full-screen aside behind the host panel, clipboard rejection, timers, down-migration default | Protocol moved to `features/collab`; aside hidden for the host panel; caught errors; timers cleared; `DROP DEFAULT` |
| Bug | — | Waiting room flickered after granting camera access | Stable `usePreviewTracks` error callback (it re-created tracks every render); browser e2e asserts no repeated `getUserMedia` |

## Phase 8 — Camera effects (backgrounds and face masks)

- security-performance-reviewer: **PASS WITH CHANGES** (no High).
- backend-architect: **APPROVE WITH CHANGES**.

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| BA8-1 | **High** | A frame that threw drew the raw camera, even with a background on | Failing frames send the privacy blur when a background is set |
| BA8-2 | Med | Worker load failures are async, so the interval fallback never ran | `onerror` switches to `setInterval` |
| BA8-3 | Med | A camera first turned on mid-meeting was published before the processor attached | Processor supplied through `room.options.videoCaptureDefaults`; e2e covers it |
| BA8-4 | Med | Second `<video>` decoding the same track | Reuses the element LiveKit passes in |
| BA8-5 / SP8-3 | Med | Both models at 30 fps on full-size frames on the main thread | Models alternate (15 fps each); segmentation input is 256 px wide |
| BA8-6 | Med | Per-processor timestamps could go backwards on the shared models | One monotonic clock in `vision.ts` |
| SP8-1/2 | Med | 14.7 MB wasm gzipped per request (CPU DoS); prod had no compression | Precompressed `.gz` at build, `gzip_static`, `gzip off` for `/vision/`, 4 connections per IP |
| SP8-4 | Med | Blocked `localStorage` threw at import and broke prejoin and the room | Storage access wrapped; choices just aren't remembered |
| SP8-5..8 | Low | `save`/`restore` not exception-safe; stale mask after device switch; shared default processor instance; decompression-bomb images | `try/finally`; mask reset on restart; a fresh processor per capture (getter); 40 MP cap and downscale to 1920 px |
| SP8-9/10/11 | Low | Canvases kept after destroy; unversioned 7-day cache; no fetch timeout | Canvases freed; versioned folder, immutable cache; 120 s timeout |
| BA8-7..13 | Low | Store injected for testability, swatch typing, stale toggle closure, warning on unmount, ADR missing | Fixed; ADR-019 added |
| BA8-9 | Low | Some mask/scene drawing functions exceed 15 lines | **Kept**: straight-line drawing code; splitting adds no clarity |
| BA8-10 | Low | Hand-written persistence instead of zustand `persist` | **Kept**: a few lines, and storage errors must be swallowed explicitly |
| SP8-9b | Low | MediaPipe instances never closed after leaving | **Accepted**: reused on the next meeting in the same tab; freed when the tab closes |
| Bug | — | Mic and camera buttons in the control bar had no accessible name | Labelled "Microphone" and "Camera" |

