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
| SP-10 | Low | Data containers writable | `read_only` + tmpfs for Postgres/Redis. **Accepted:** images pinned by exact tag, digests deferred to release pipeline |
| SP-11 | Low | No idle-in-tx timeout | `idle_in_transaction_session_timeout=10s` |
| SP-12 | Low | Raw IPs in Redis keys | Keys are HMAC'd |
| Handoffs | — | Recover after headers; detached readiness probes; cancel ≠ Redis error | All fixed |
| SP-13 | Info | SSE write deadline / shutdown hook | Implemented in Phase 4 (admission hub) |
