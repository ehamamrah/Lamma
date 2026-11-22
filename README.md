# Lamma

Instant, anonymous video meetings in the browser. Create a meeting, share the
link, and talk. Nobody needs an account or an app.

LiveKit carries all audio and video. A Go modular monolith handles meetings,
permissions, chat and moderation. The React app runs in any modern desktop or
mobile browser.

- **Backend:** Go 1.27, chi, pgx + sqlc, PostgreSQL 17, Redis 8, slog,
  OpenTelemetry, Prometheus, golang-migrate
- **Frontend:** React 19, TypeScript, Vite, Tailwind CSS v4, Zustand,
  React Router, LiveKit React SDK, MediaPipe (camera effects)
- **Infrastructure:** Docker Compose, LiveKit server with TURN, Nginx

---

## Contents

- [Features](#features)
- [Quick start](#quick-start)
- [Using Lamma](#using-lamma)
- [Configuration](#configuration)
- [Testing from a phone](#testing-from-a-phone)
- [Deploying to production](#deploying-to-production)
- [Development](#development)
- [Tests](#tests)
- [How it works](#how-it-works)
- [Browser support](#browser-support)
- [Troubleshooting](#troubleshooting)
- [Documentation](#documentation)

---

## Features

### Meetings

- **One-click meetings.** You get a random code (`xxxx-xxxx-xxxx`, 60 bits)
  and a shareable link, and you become the host.
- **No accounts.** People join with a display name only.
- **Optional protection:** a meeting password, a waiting room, or both.
- **No participant limit.** Anyone with the link can join. The host decides
  when to stop that by locking the meeting or turning on the waiting room.
- **Meetings expire** after 24 hours by default (configurable) and are
  cleaned up automatically.

### In the call

- Audio, video and screen sharing, with device selection and a camera preview
  before you join.
- Chat, and a list of everyone in the meeting.
- **Emoji reactions** that float over the video for everyone.
- **Camera effects** (see below).
- **A shared whiteboard** (see below).

### Host controls

- Admit or deny people in the waiting room, one at a time or all at once.
- Mute or remove a participant.
- **Lock the meeting** so nobody new can join, or switch the waiting room on
  or off mid-meeting.
- End the meeting for everyone.

### Whiteboard

The host opens the whiteboard from the control bar and chooses a mode:

- **Only me:** only the host draws; everyone else watches.
- **Collaborative:** guests tap **Request to draw**. The host approves or
  denies each request and can revoke drawing rights at any time.

People who join later see everything drawn so far. Pens come in several
colours and widths, with undo. The host can also clear the board.

### Recording

The host can record from a desktop browser. Lamma records the meeting tab
with everyone's audio and **downloads the file to the host's computer**, so
nothing is stored on the server. While a recording runs, everyone sees a
**Recording** indicator.

### Camera effects

Open **Effects** in the camera preview before joining, or from the control
bar during a meeting.

- **Backgrounds:** light or strong blur, painted scenes (Studio, Sunset,
  Ocean, Forest, Night sky) or **your own image**.
- **Face masks:** sunglasses, cat, party hat, mustache, clown and crown. The
  masks follow your head as you move.

All processing happens **in your browser**. Your uploaded image never leaves
your device. If you choose a background before joining, it is applied
before your camera goes live, so others never see your real room. While the
background model is still loading, Lamma sends a heavily blurred picture
rather than your raw camera. Lamma remembers your choice for next time (but
not an uploaded image).

### Phones and tablets

- A compact control bar with a **More** sheet for secondary actions.
- People and the whiteboard fit small screens; video tiles scroll
  sideways while someone shares their screen or the whiteboard.
- A **Tap to turn on sound** button appears when the browser blocks
  autoplay.

### Security and privacy

- No accounts, no tracking, and meeting codes are never written to logs.
- Server-side authorization: the browser's idea of who is host is for
  display only. Every host action is checked by the API and LiveKit.
- Rate limits on creating, joining, password attempts, chat and moderation.
- Strict Content-Security-Policy. Every script, model and font is served
  from our own origin, with no third-party CDNs.
- Read-only, non-root containers.

See [docs/security.md](docs/security.md) for the full threat model.

---

## Quick start

You need **Docker** with **Compose v2**. That's all.

```sh
git clone https://github.com/ehamamrah/Lamma.git
cd Lamma
docker compose up --build
```

The first build takes a few minutes. When every service is healthy:

1. Open <http://localhost:8080>.
2. Enter your name and click **New meeting**.
3. Check your camera and microphone, then click **Join now**.
4. Click **Copy link** and open it in another browser or a private window
   to join as a guest.

Every setting has a development default, so you don't need a `.env` file.
To change something, copy `.env.example` to `.env` and edit it.

Stop the stack with `docker compose down`. Add `-v` to also delete the
database.

### Ports used locally

| Port | Purpose |
| --- | --- |
| 8080/tcp | Web app and API (Nginx) |
| 7880/tcp | LiveKit signalling (WebSocket) |
| 7881/tcp | WebRTC over TCP (fallback) |
| 7882/udp | WebRTC media |
| 3478/udp | TURN |

---

## Using Lamma

**Start a meeting.** On the home page, enter your name and click **New
meeting**. Optionally give it a title, a password (8+ characters) or turn on
the waiting room.

**Invite people.** Share the link (`/m/xxxx-xxxx-xxxx`) or the code. Guests
can paste either into **Join a meeting** on the home page.

**Join.** Guests enter a name (and the password, if there is one). With a
waiting room, they wait until the host admits them.

**Control bar.** Microphone, camera, reactions, chat and leave are always
visible. Screen share, people, effects and copy link sit next to them on
desktop and under **More** on phones. Hosts also get **Whiteboard**,
**Record** and **Host controls**. A badge shows waiting guests and
drawing requests.

**Host controls.** Admit or deny waiting guests, lock the meeting, toggle the
waiting room, mute or remove people, or end the meeting for everyone.

---

## Configuration

All settings are environment variables. [.env.example](.env.example) lists
every one with its default. The ones you are most likely to change:

| Variable | Default | Purpose |
| --- | --- | --- |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | The URL people open |
| `WEB_PORT` | `8080` | Host port for the web app |
| `LIVEKIT_PUBLIC_URL` | `ws://localhost:7880` | LiveKit URL as seen by browsers |
| `LIVEKIT_NODE_IP` | `127.0.0.1` | IP LiveKit advertises for media (LAN IP for phones, public IP in production) |
| `CSP_CONNECT_SRC` | `ws://localhost:7880 http://localhost:7880` | Must include `LIVEKIT_PUBLIC_URL` |
| `MEETING_DEFAULT_TTL` | `24h` | How long a meeting lives |
| `MEETING_MAX_TTL` | `168h` | Longest allowed lifetime |
| `RATE_LIMIT_*` | see file | Per-IP and per-meeting rate limits |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

The full reference is in [docs/operations.md](docs/operations.md).

---

## Testing from a phone

Browsers only allow the camera and microphone on **HTTPS** pages (or
`localhost`). A phone opening `http://192.168.x.x:8080` sees the app but
cannot use its camera. Either:

- deploy with TLS (see below), or
- put an HTTPS tunnel in front of port 8080 (for example
  `cloudflared tunnel --url http://localhost:8080` or ngrok). Set
  `PUBLIC_BASE_URL` to the tunnel URL. LiveKit's media ports still need to
  be reachable from the phone, so on the same Wi-Fi set
  `LIVEKIT_NODE_IP=<your LAN IP>`, and set `LIVEKIT_PUBLIC_URL` and
  `CSP_CONNECT_SRC` to match.

See [docs/operations.md](docs/operations.md#livekit-networking) for the
details.

---

## Deploying to production

In short:

1. Get TLS certificates for the app domain, the RTC (LiveKit) domain and
   TURN.
2. Create `.env` from `.env.example`. Generate every secret with
   `openssl rand -base64 48`, and set the production values (`APP_ENV`,
   `https`/`wss` URLs, `COOKIE_SECURE=true`, `TRUSTED_PROXY_CIDRS`,
   TURN settings).
3. Open the media ports: 443/tcp, 7882/udp, 7881/tcp, 3478/udp, 5349/tcp.
4. Start it:

   ```sh
   docker compose -f compose.yaml -f compose.prod.yaml up -d --build
   ```

The API refuses to start in production with weak or development secrets,
insecure cookies or non-HTTPS URLs.

**Upgrading:** database migrations run automatically on every `up`. Some
releases need the API stopped first; check
[docs/operations.md](docs/operations.md#database-migrations) before
upgrading.

---

## Development

### Prerequisites

- Docker with Compose v2
- Node.js 24+ (frontend)
- Go is optional: `scripts/go.sh` runs Go 1.27 in a container

### Frontend with hot reload

Start the backend stack, then the Vite dev server:

```sh
docker compose up -d --build
cd frontend
npm ci
npm run dev          # http://localhost:5173, proxies /api to the stack
```

The first `npm run dev` or `npm run build` downloads the camera-effect models
(about 4 MB) from Google's model storage into `frontend/public/vision/` and
checks their SHA-256 hashes. The script is `scripts/vision-assets.mjs`.

### Common tasks

| Task | Command |
| --- | --- |
| Go unit tests | `scripts/go.sh go test -race ./...` |
| Go formatting | `scripts/go.sh gofmt -l .` |
| Go lint | `docker run --rm -v "$PWD/backend:/src" -w /src golangci/golangci-lint:v2.15.0 golangci-lint run` |
| Regenerate SQL code (after editing `backend/db/queries`) | `scripts/sqlc.sh` |
| Frontend lint, types, unit tests | `cd frontend && npm run lint && npm run typecheck && npm test` |
| Frontend formatting | `cd frontend && npm run format` |
| Logs | `docker compose logs -f api` |

### Conventions

- No Go function longer than 15 statements; golangci-lint enforces it.
- Backend services import no HTTP, database, Redis or LiveKit packages;
  each module declares the interfaces it needs.
- API changes update the Go handler, the TypeScript client
  (`frontend/src/shared/api`) and `api/openapi.yaml` together.
- After each change set, run the `backend-architect` and
  `security-performance-reviewer` reviews (`.claude/agents/`) and record the
  results in [docs/review-log.md](docs/review-log.md).

More in [docs/development.md](docs/development.md).

---

## Tests

| Suite | What it covers | How to run |
| --- | --- | --- |
| Go unit | Services, handlers, rate limits, tokens | `scripts/go.sh go test -race ./...` |
| Go integration + API e2e | Real Postgres, Redis and LiveKit | see [docs/development.md](docs/development.md#backend) |
| Frontend unit | Stores, protocols, geometry, components (Vitest) | `cd frontend && npm test` |
| Browser e2e | Two real browsers with fake cameras: joining, chat, moderation, waiting room, reactions, whiteboard, phone layout, camera effects (Playwright) | `cd frontend && npx playwright install chromium && npm run test:e2e` |

Browser e2e runs against the stack on `:8080`, so start it first with
`docker compose up -d --build`.

CI (GitHub Actions) runs all of these plus dependency and image scanning on
every push.

---

## How it works

```text
 Browser (React SPA)
   │  REST /api/v1 (cookies, CSRF-checked)          WebRTC media + data
   ▼                                                     │
 Nginx ── static files, security headers ──┐            ▼
   │                                        │      LiveKit SFU (+TURN)
   ▼                                        │            ▲
 Go API (modular monolith) ─────────────────┼── room/token management
   │            │                           │
 PostgreSQL   Redis (rate limits, chat, waiting-room events)
```

- The **API** owns meetings, participants, admission, moderation and chat.
  It issues short-lived LiveKit tokens with only the permissions each person
  needs.
- **LiveKit** carries audio, video and screen share. Small LiveKit data
  messages carry reactions and the whiteboard. Every receiver checks the
  sender's server-assigned role, so guests cannot pretend to be the host.
- **Camera effects** are a LiveKit track processor. Camera frames are drawn
  onto a canvas with MediaPipe's person segmentation and face landmarks, and
  the canvas is published in place of the raw camera.
- **Recording** happens in the host's browser (screen capture plus a mix of
  everyone's audio), so no media is stored on the server.

Repository layout:

```text
backend/            Go API (modular monolith)
  cmd/api/          entrypoint: serve | migrate up|down | healthcheck
  internal/         modules: meeting, participant, access, moderation, chat,
                    admission, realtime, maintenance, health, platform/*
  db/               migrations and sqlc queries
  integration/ e2e/ tests against Postgres and the running stack
frontend/           React SPA, Nginx config, Playwright e2e
  src/features/     prejoin, room, chat, moderation, reactions, whiteboard,
                    collab, recording, effects, …
api/openapi.yaml    API reference
deploy/             Postgres init, production Nginx template
docs/               architecture, operations, security, performance, reviews
PLAN.md             design, decisions and delivery plan
```

The architecture, data flow and key decisions are documented in
[docs/architecture.md](docs/architecture.md) and [PLAN.md](PLAN.md).

---

## Browser support

| Feature | Chrome / Edge | Firefox | Safari (macOS, iOS) | Android Chrome |
| --- | --- | --- | --- | --- |
| Meetings, chat, reactions, whiteboard | ✅ | ✅ | ✅ | ✅ |
| Screen sharing | ✅ | ✅ | ✅ macOS only | — |
| Camera effects | ✅ | ✅ | ✅ (recent versions) | ✅ |
| Recording (host) | ✅ | ✅ | ✅ macOS only | — |

Camera effects use the CPU. On older phones, prefer blur or a mask over a
full virtual background if the video stutters.

---

## Troubleshooting

| Symptom | Likely cause and fix |
| --- | --- |
| Video doesn't connect, but the page loads | The media ports (7882/udp, 7881/tcp, 3478/udp) are blocked, or `LIVEKIT_NODE_IP` is wrong for where the browser is |
| A phone can't use its camera | The page isn't HTTPS. See [Testing from a phone](#testing-from-a-phone) |
| "Tap to turn on sound" appears | The browser blocked autoplay. Tap it once |
| Effects say "could not load" | The `/vision/` files are missing. Rebuild the web image (`docker compose up -d --build web`) |
| Recording button is missing | Recording needs a desktop browser with screen capture |
| "Too many requests" when creating meetings | The creation rate limit (`RATE_LIMIT_CREATE`, default 10/hour per IP) |

---

## Documentation

- [Architecture](docs/architecture.md): modules, data flow, key decisions
- [Operations](docs/operations.md): deployment, LiveKit networking,
  migrations, environment variables
- [Development](docs/development.md): local setup and conventions
- [Security](docs/security.md) and [Performance](docs/performance.md)
- [API reference](api/openapi.yaml)
- [Design plan](PLAN.md) and [review log](docs/review-log.md)
