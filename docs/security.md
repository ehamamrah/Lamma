# Security

The threat model is anonymous internet users with a browser and scripts.
There are no accounts, so access to a meeting is controlled by knowing its
code, with the host's moderation powers on top.

## Identity and authorization

- **Meeting codes:** 60 random bits (Crockford base32). Unknown, ended and
  expired meetings all return the same 404, and lookups are rate limited per
  IP. Logs and LiveKit use the internal UUID, never the code.
- **Sessions:** a 256-bit random token in an HttpOnly, SameSite=Strict
  cookie (`__Secure-` and Secure in production), scoped to
  `/api/v1/meetings/{code}`. Only its SHA-256 hash is stored.
- **Roles:** read only from the session row. The client never supplies a
  role. Host routes pass a middleware check, and the service checks again.
  Every target lookup is scoped to the caller's meeting.
- **CSRF:** SameSite=Strict cookies, an Origin / `Sec-Fetch-Site` check on
  unsafe methods, and JSON-only request bodies.
- **LiveKit tokens:** valid for 2 minutes, for one room only, with an
  identity set by the server. No admin or metadata-update permissions.
  Host powers exist only in the API. Rooms are never
  auto-created. Every `participant_joined` webhook is checked against the
  database, intruders are evicted, and a 5-minute anti-entropy sweep catches
  any webhooks that were lost.

## Peer-to-peer collaboration

Reactions and the whiteboard travel as LiveKit data messages, so tokens
allow data publishing. LiveKit stamps every message with the sender's
identity and server-signed metadata (which holds the role and which
participants cannot change), so receivers can trust who sent what:

- **Chat** is accepted only from the server (no participant sender), so
  peers cannot spoof chat messages.
- **Host state** (recording flag, whiteboard mode, who may draw) is accepted
  only from a participant whose role is `host`.
- **Strokes** are applied only if the host's current state lets that sender
  draw (the host always can; guests only in collaborative mode after the
  host approves them). Clearing the board is host-only, and a stroke can
  only be extended or undone by its author.
- **Validation:** every message is parsed against a strict schema: emojis
  and colours from fixed lists, coordinates in 0..1, and bounded point
  counts, stroke counts and id lengths. Reactions and guest-to-host messages
  are rate limited per sender on the receiving side.
- **Recording** happens in the host's browser (screen capture plus
  MediaRecorder) and is downloaded locally; nothing is uploaded. While it
  runs, every participant sees a "Recording" indicator.

- **Flood limits** (receive side): strokes 40 messages/s per drawer; board
  budgets of 2,000 points per stroke, 300 strokes per author and 200,000
  points in total; snapshot requests 6/min per guest and 5/s overall (the
  host caches the serialised board); reactions 5 per 3 s per sender and 10/s
  overall. The board redraws at most once per animation frame.

Accepted risks:

- Any admitted guest can publish data, so a malicious client can still send
  junk messages. Receivers drop them, but they cost bandwidth and parsing.
  The host's remove action is the backstop. A stricter design would grant
  data publishing per participant from the API when the host approves.
- There is no participant cap (product requirement). Joins are limited per
  IP and per IP per meeting, so many IPs can grow an open meeting. Hosts
  close it with lock or gate it with the waiting room; operators should
  size LiveKit nodes for their largest meetings.
- The recording indicator relies on the host's browser announcing it. A
  modified client could record without showing it, as anyone can with a
  screen recorder.

## Abuse controls

- **Passwords:** argon2id (19 MiB, t=2) behind a concurrency gate that
  returns 503 when full. The attempt limit is spent before argon2 runs.
  Passwords need at least 8 characters. There is no meeting-wide lockout,
  so outsiders cannot lock real guests out.
- **Rate limits:** distributed limits (Redis GCRA) per IP (IPv6 grouped by
  /64), per IP per meeting, and per session. Fail-closed for creation and
  passwords; fail-open with a per-client in-memory fallback for everything
  else.
- **Caps:**
  - Waiting room: 3 per origin, 50 per meeting.
  - Seats: claimed at token time with leases; at most 3 unconnected seats
    per origin.
  - Meetings have no participant cap; join rate limits per IP bound
    scripted joins, and the host can lock or gate the meeting.
  - SSE streams: 3 per IP and 2,000 per replica.
- **Trusted proxies:** `X-Forwarded-For` is trusted only from Nginx's fixed
  IP (`TRUSTED_PROXY_CIDRS`), and Nginx overwrites the header rather than
  appending to it.

## Input and output

- **Validation:** strict JSON decoding (unknown fields rejected, 16 KiB
  limit). Names, titles and chat are NFC-normalised, and control and bidi
  override characters are stripped.
- **Errors:** RFC 9457 problem responses with stable codes. Internal
  errors are never exposed to clients.
- **Headers:** CSP, `frame-ancestors 'none'`, `X-Frame-Options: DENY`,
  `Referrer-Policy: no-referrer` (meeting URLs are capabilities), HSTS in
  production, and a limited `Permissions-Policy`.
- **Frontend:** no `innerHTML`. Chat only accepts data messages sent by the
  server. Tokens live only in memory.

## Camera effects

- Frames are processed in the browser only. Uploaded background images are
  decoded locally and never sent or saved.
- While a background is selected, nothing unprocessed is published. If the
  segmentation model hasn't loaded yet, a heavily blurred frame is sent.
- The MediaPipe runtime and models are self-hosted under `/vision/`, so no
  third-party origins are added to the CSP. `script-src` gains
  `'wasm-unsafe-eval'`, which lets WebAssembly compile but still forbids
  JavaScript `eval`. Models are pinned by SHA-256 at build time, and the
  wasm comes from the pinned npm package.

## Infrastructure

- Containers run as non-root with a read-only root filesystem,
  `no-new-privileges` and all capabilities dropped. Base images are pinned
  by digest.
- The Postgres app role cannot change the schema, and the audit log is
  append-only for it.
- The Redis password is kept out of the process arguments.
- `/internal` (webhooks) and `/metrics` are never exposed through Nginx.
- The TURN relay cannot reach private or internal networks.
- Production guards stop startup on dev or weak secrets, insecure cookies,
  missing proxy configuration, or non-TLS public URLs.
- CI runs govulncheck, npm audit, Trivy (filesystem and images) and
  Gitleaks.

## Known limitations (accepted for the MVP)

- A removed guest can rejoin with a new session. The controls for this are
  locking the meeting and the waiting room (PLAN A3).
- There is no CAPTCHA (PLAN A8). The join and create handlers keep a single
  place where one can be added.
- The CSP keeps `style-src 'unsafe-inline'` for LiveKit components.
