# Operations

## Services

| Service | Image | Notes |
| --- | --- | --- |
| `postgres` | postgres:17-alpine (pinned by digest) | Persistent `pgdata` volume. The owner role runs migrations; `lamma_app` has data access only |
| `redis` | redis:8-alpine | No persistence. Holds rate limits, chat history and pub/sub only |
| `migrate` | `lamma/api` | One-shot `api migrate up`, run before the API starts |
| `livekit` | livekit-server v1.13.11 | SFU plus TURN. Rooms are never auto-created |
| `api` | `lamma/api` (distroless, non-root, read-only) | `:8000` HTTP and `:9090` metrics, both internal only |
| `web` | `lamma/web` (unprivileged Nginx, read-only) | Serves the SPA, proxies `/api`, sets security headers. Fixed IP `172.30.0.10` |

Start order is enforced by health checks:
postgres → migrate → redis and livekit → api → web.

## Local

```sh
docker compose up --build          # dev overlay loads automatically
docker compose logs -f api         # JSON logs
docker compose down -v             # stop and wipe data
```

## Production

1. Get TLS certificates for the app domain (`app.crt`/`app.key`) and the RTC
   domain (`rtc.crt`/`rtc.key`), plus a TURN certificate (`turn.crt`/`turn.key`),
   all in `TLS_CERT_DIR`.
2. Create `.env` from `.env.example`. Generate every secret with
   `openssl rand -base64 48`, and set `APP_ENV=production`,
   `PUBLIC_BASE_URL=https://…`, `LIVEKIT_PUBLIC_URL=wss://rtc.…`,
   `TRUSTED_PROXY_CIDRS`, `COOKIE_SECURE=true`, `LIVEKIT_USE_EXTERNAL_IP=true`,
   `LIVEKIT_TURN_DOMAIN`, and `LIVEKIT_TURN_TLS_PORT=5349`.
3. Start it:

```sh
docker compose -f compose.yaml -f compose.prod.yaml up -d --build
```

The API refuses to start in production with weak or dev secrets, insecure
cookies, missing trusted proxies, or non-https/wss public URLs.

### Scaling

The API is stateless. Run more replicas behind Nginx; Redis coordinates
rate limits and waiting-room events, and Postgres `SKIP LOCKED` keeps
maintenance work from being duplicated. LiveKit runs as a single node here.
To run LiveKit as multiple nodes, follow LiveKit's own guide (it adds its
Redis config).

## LiveKit networking

HTTP ports alone are **not** enough for video. Open:

| Port | Proto | Why |
| --- | --- | --- |
| 443 | TCP | App (Nginx) and LiveKit signalling (`wss://rtc.…` through Nginx) |
| 7882 | UDP | WebRTC media (single-port mux). This is the main media path |
| 7881 | TCP | ICE over TCP, used when UDP is blocked |
| 3478 | UDP | TURN over UDP |
| 5349 | TCP | TURN over TLS, for strict corporate networks (ideally 443 on a separate IP) |

- **Local:** `NODE_IP=127.0.0.1` with `use_external_ip: false`. For LAN
  testing, set `LIVEKIT_NODE_IP` and `LIVEKIT_PUBLIC_URL` to your LAN IP.
- **Production:** `LIVEKIT_USE_EXTERNAL_IP=true` (STUN discovery), or set
  `LIVEKIT_NODE_IP` to the public IP.
- **TURN restrictions:** TURN may not relay to private, loopback,
  link-local, CGNAT or the stack's own network (`deny_peer_cidrs` in
  `compose.yaml`).
- **Webhooks:** LiveKit posts webhooks to
  `http://api:8000/internal/livekit/webhook` on the internal network. Nginx
  never exposes `/internal`.
- **Tuning:** for production, raise the kernel UDP buffer:
  `sysctl -w net.core.rmem_max=5000000`.

## Database migrations

Migrations are embedded in the API binary (`backend/db/migrations`) and run
by the `migrate` service on every `up`.

```sh
docker compose run --rm migrate migrate up     # apply
docker compose run --rm migrate migrate down   # roll back one step
```

To add a migration, create the next numbered pair
`NNNNNN_name.up.sql` / `.down.sql`, then run `scripts/sqlc.sh` if any
queries change.

Migrations that drop or rename columns break API replicas still running the
old code (sqlc expands `SELECT *` into explicit columns). For those, stop the
API, migrate, then start the new version. `000002_unlimited_participants`
(drops `meetings.max_participants`) is one of them:

```sh
docker compose stop api
docker compose up -d --build   # migrate runs before the new api starts
```

## Environment variables

Defaults are in [.env.example](../.env.example) and
`backend/internal/platform/config`.

| Variable | Default (dev) | Purpose |
| --- | --- | --- |
| `APP_ENV` | `production` in the binary, `development` in compose | Turns on HSTS and the production guards |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Meeting URLs and the CSRF origin |
| `DATABASE_URL` / `DATABASE_MIGRATE_URL` | — | App role / owner role |
| `DB_MAX_CONNS` | 20 | pgx pool size |
| `REDIS_URL` | — | Redis connection |
| `LIVEKIT_URL` / `LIVEKIT_PUBLIC_URL` | `http://livekit:7880` / `ws://localhost:7880` | Server API URL / browser URL |
| `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET` | dev values | Secret is at least 32 bytes in production |
| `LIVEKIT_TOKEN_TTL` | `2m` | JWT lifetime, which is also the seat lease (30s–5m) |
| `LIVEKIT_CALL_TIMEOUT` | `3s` | Per LiveKit API call |
| `ORIGIN_HASH_KEY` | dev value | HMAC key for client-origin hashes and rate-limit keys |
| `COOKIE_SECURE` | `false` in dev | Must be `true` in production |
| `TRUSTED_PROXY_CIDRS` | `172.30.0.10/32` | Peers whose `X-Forwarded-For` is trusted |
| `CORS_ALLOWED_ORIGINS` | empty | Extra allowed origins (https only in production) |
| `MEETING_DEFAULT_TTL` / `MEETING_MAX_TTL` / `MEETING_RETENTION` | `24h` / `168h` / `168h` | Meeting lifetime and data retention |
| `RATE_LIMIT_*` | see `.env.example` | `N/period`, e.g. `10/m`, `5/15m` |
| `ARGON2_MAX_CONCURRENT` | CPU count | Password-hashing slots |
| `BREAKER_FAILURE_THRESHOLD` / `BREAKER_OPEN_TIMEOUT` | 5 / `30s` | LiveKit circuit breakers |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | Turns on trace export (host:port) |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

## Observability

- **Logs:** JSON on stdout, with `request_id`, `trace_id`, `meeting_id` (the
  internal UUID) and `participant_id`. Secrets, codes and names are redacted.
- **Metrics:** `api:9090/metrics` (internal). `meetings_active` and
  `participants_in_room` are global values, so aggregate them with `max()`.
- **Health:** `/health/live` (process only) and `/health/ready` (fails only
  on Postgres or while draining; Redis and LiveKit status is reported in the
  body).
