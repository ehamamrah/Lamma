# Lamma

Instant, anonymous video meetings in the browser. Create a meeting, share the
link, talk. There are no accounts. LiveKit carries all media; a Go modular
monolith handles meetings, permissions, chat and moderation.

- **Backend:** Go 1.27, chi, pgx + sqlc, PostgreSQL 17, Redis 8, slog,
  OpenTelemetry, Prometheus, golang-migrate
- **Frontend:** React 19, TypeScript, Vite, Tailwind CSS v4, shadcn-style UI,
  Zustand, React Router, LiveKit React SDK
- **Infrastructure:** Docker Compose, LiveKit server with TURN, Nginx

## Features

- Create a meeting with one click. You get a 60-bit random code
  (`xxxx-xxxx-xxxx`) and a shareable URL, and you become the host.
- Join by name, with an optional meeting password and an optional waiting room.
- Audio, video, screen share, a participant list and chat.
- No participant limit: anyone with the link can join. The host can lock
  the meeting (no new joiners) or turn on the waiting room (approval only).
- Host controls: admit, deny or deny all waiting; mute; remove; lock;
  toggle the waiting room; end the meeting for everyone.
- Emoji reactions that float over the video.
- A shared whiteboard. The host draws alone ("Only me") or makes it
  collaborative, where guests ask to draw and the host approves each one.
- Recording (host, desktop browsers): records the meeting tab with everyone's
  audio and downloads it as a file. Everyone sees a "Recording" indicator.
- Works on phones: a compact control bar with a "More" sheet, and a tap to
  enable sound when the browser blocks autoplay.
- Meetings expire (24 h by default, configurable) and are cleaned up
  automatically.

## Quick start

You need Docker with Compose v2.

```sh
docker compose up --build
```

Open <http://localhost:8080>, create a meeting, and open the link in a second
browser (or a private window) to join.

Every setting has a development default, so no `.env` file is needed locally.
To customise, copy `.env.example` to `.env`.

### Ports used locally

| Port | Purpose |
| --- | --- |
| 8080/tcp | Web app and API (Nginx) |
| 7880/tcp | LiveKit signalling (WebSocket) |
| 7881/tcp | WebRTC over TCP (fallback) |
| 7882/udp | WebRTC media |
| 3478/udp | TURN |

To test from a phone on your LAN, set `LIVEKIT_NODE_IP=<your LAN IP>` and
`LIVEKIT_PUBLIC_URL=ws://<your LAN IP>:7880`. See
[docs/operations.md](docs/operations.md).

## Repository layout

```text
backend/            Go API (modular monolith)
  cmd/api/          entrypoint: serve | migrate up|down | healthcheck
  internal/         modules: meeting, participant, access, moderation, chat,
                    admission, realtime, maintenance, health, platform/*
  db/               migrations + sqlc queries
  integration/ e2e/ tests against Postgres / the running stack
frontend/           React SPA, Nginx config, Playwright e2e
api/openapi.yaml    API reference
deploy/             Postgres init, production Nginx template
docs/               architecture, operations, security, performance, reviews
PLAN.md             design, decisions and delivery plan
```

## Development

| Task | Command |
| --- | --- |
| Go tests | `scripts/go.sh go test -race ./...` (runs Go in Docker) |
| Go lint | `docker run --rm -v "$PWD/backend:/src" -w /src golangci/golangci-lint:v2.15.0 golangci-lint run` |
| Regenerate SQL code | `scripts/sqlc.sh` |
| Frontend dev server | `cd frontend && npm ci && npm run dev` (needs the stack running) |
| Frontend checks | `npm run lint && npm run typecheck && npm test` |
| Integration and API e2e | see the header of `backend/integration/postgres_test.go` |
| Browser e2e | `cd frontend && npx playwright install chromium && npm run test:e2e` |

More detail is in [docs/development.md](docs/development.md).

## Documentation

- [Architecture](docs/architecture.md): modules, data flow, key decisions
- [Operations](docs/operations.md): deployment, LiveKit networking,
  migrations, environment variables
- [Security](docs/security.md) and [Performance](docs/performance.md)
- [API reference](api/openapi.yaml)
- [Design plan](PLAN.md) and [review log](docs/review-log.md)
