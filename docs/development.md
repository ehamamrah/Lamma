# Development

## Prerequisites

You need Docker (Compose v2) and, for the frontend, Node 24 or later.
Go is optional: `scripts/go.sh` runs Go 1.27 in a container.

## Backend

```sh
scripts/go.sh go test -race ./...     # unit tests
scripts/go.sh gofmt -l .               # formatting
scripts/sqlc.sh                        # regenerate after editing db/queries
```

Integration and API end-to-end tests run against the Docker stack:

```sh
docker compose up -d --build
GO_DOCKER_ARGS="--network lamma_lamma -e LAMMA_API=http://api:8000 \
  -e LAMMA_TEST_DATABASE_URL=postgres://lamma_app:lamma-dev-only-app@postgres:5432/lamma?sslmode=disable" \
  scripts/go.sh go test -race -tags=integration,e2e ./integration/... ./e2e/...
```

## Frontend

```sh
cd frontend
npm ci
npm run dev        # http://localhost:5173, proxies /api to the stack on :8080
npm run lint && npm run typecheck && npm test
npx playwright install chromium && npm run test:e2e   # two browsers, fake media
```

## Conventions

- **Backend size limit:** no Go function over 15 statements. golangci-lint
  enforces this with `funlen`.
- **Backend layering:** service code imports no HTTP, pgx, Redis or LiveKit
  packages. Each module declares the interfaces it consumes.
- **Frontend:** stage transitions belong in `features/meeting/session.ts`.
  Roles in the UI are for display only.
- **API changes:** update the Go handler, the TypeScript client in
  `frontend/src/shared/api` and `api/openapi.yaml` in the same change.
- **Reviews:** run the `backend-architect` and `security-performance-reviewer`
  sub-agents (`.claude/agents/`) after each change set and record the
  results in `docs/review-log.md`.
