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
  identity set by the server. No admin, data-publish or metadata-update
  permissions. Host powers exist only in the API. Rooms are never
  auto-created. Every `participant_joined` webhook is checked against the
  database, intruders are evicted, and a 5-minute anti-entropy sweep catches
  any webhooks that were lost.

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
  - Admitted guest rows: at most 4× capacity.
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
