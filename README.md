# PRobot

Marketing & PR hub: multi-platform publishing, cross-links, crawlers, AI agents, analytics graph.

## Stack

- Go API (`chi`) + Asynq workers + crawler
- Postgres (goose migrations) + Redis + MinIO
- React + Vite + TypeScript + Tailwind + Babylon.js graph

## Quick start (Docker)

```bash
cp .env.example .env   # add OPENAI_API_KEY if you want AI drafts
docker compose up -d --build
```

- Web: `http://localhost:3000`
- API: `http://localhost:8080`
- MinIO console: `http://localhost:9001`

Postgres is on `localhost:5433`, Redis on `6379`. Compose runs migrate, API, worker, crawler, and the UI.

## Local Go (infra only)

```bash
cp .env.example .env
docker compose up -d postgres redis minio minio-init
export DATABASE_URL='postgres://probot:probot@localhost:5433/probot?sslmode=disable'
goose -dir migrations postgres "$DATABASE_URL" up
go run ./cmd/api
# other terminals:
go run ./cmd/worker
go run ./cmd/crawler
cd apps/web && npm install && npm run dev
```

## MVP API

- `POST /api/v1/auth/register` — create user + workspace
- Brands, campaigns, content, channels, publications
- `POST .../publications/{id}/enqueue` — publish via worker
- `GET /api/v1/playbooks` + `POST .../campaigns/{id}/apply-playbook`
- Short links: `POST .../short-links`, public redirect `GET /r/{code}`
- Agents CRUD + `POST .../agents/{id}/run`
- Adapters: Telegram, VK, Bluesky, X, Reddit, Mastodon + custom webhook/manual
- Crawl sources + mentions + `POST .../mentions/{id}/draft-reply` + `POST .../mentions/{id}/objection`
- AI research: `POST .../brands/{id}/ai-research` (OpenAI, Claude/Anthropic, Grok/xAI, Gemini)
- `GET .../ai/providers` + `POST .../agents/seed-research`
- Analytics: `GET .../analytics/summary`, `POST .../analytics/advise`, stats poll every 15m in worker
- Channel health auto warn/pause after failures; `PATCH .../channels/{id}/health`
- Brand brain: `PATCH .../brands/{id}` (tone/CTA) injected into AI runs via `brand_id`
- Karma/community: like/comment/follow/share/link-exchange queue with approval + daily caps
  - `POST .../engagement/tasks`, `.../approve`, `.../draft`
  - `GET .../engagement/karma`, partners CRUD
- `GET .../campaigns/{id}/graph` — nodes/edges for Babylon
