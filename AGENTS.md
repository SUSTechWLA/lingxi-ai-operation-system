# AGENTS.md

This repository is split by runtime boundary:

```text
frontend/       # React + Electron UI
local-backend/  # local desktop agent, no DB/Docker dependency
cloud-backend/  # Go AIOS Core cloud backend
```

Read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) first for the full architecture.

## Local Runtime

The local runtime is for desktop users. It should not depend on PostgreSQL, Redis, Kafka, MinIO, Docker, or LLM API keys.

```bash
bash scripts/start-local-backend.sh
bash scripts/start-frontend.sh
```

Local data lives under the OS application data directory and includes cache, projects, artifacts, logs, and diagnostics.

## Cloud Runtime

The cloud backend owns LLM/API integration, remote configuration, orchestration, persistence, cloud logs, and diagnostics analysis.

```bash
cd cloud-backend
cp .env.example .env
docker compose up -d
go build -o build/tangying-ai-os cmd/tangying-ai-os/main.go
./build/tangying-ai-os
```

Cloud deployment docs: [docs/CLOUD_DEPLOYMENT.md](docs/CLOUD_DEPLOYMENT.md).

## Frontend

```bash
cd frontend
npm install
npm run dev
```

Build the desktop app with the cloud API base configured:

```bash
VITE_CLOUD_API_BASE=https://your-cloud.example.com/api \
TANGYING_CLOUD_API_BASE=https://your-cloud.example.com/api \
bash scripts/build-local-desktop.sh
```

## Verification

```bash
cd local-backend && go test ./...
cd ../cloud-backend && go test ./...
cd ../frontend && npm run build
```

## Boundary Rules

- Do not add database, Docker, Kafka, Redis, or MinIO dependencies to `local-backend`.
- Do not store LLM API keys in the local desktop package.
- Local logs must be uploaded only after user action/authorization.
- Keep cloud business orchestration in `cloud-backend`.
