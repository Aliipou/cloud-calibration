# Cloud Calibration Platform

A production-quality calibration management platform built for **Beamex Oy Ab**, targeting full **ISO 17025** compliance. Written in Go with PostgreSQL, it provides a REST API, event-sourced audit log, compliance engine, and a real-time web dashboard.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     Cloud Calibration API                        │
│                    (Go 1.22 + Gin HTTP)                          │
│                                                                  │
│  ┌──────────────────┐    ┌──────────────────────────────────┐   │
│  │  REST Handlers   │    │     Calibration Service           │   │
│  │  /api/v1/...     │───▶│  ISO 17025 Compliance Engine     │   │
│  └──────────────────┘    │  · Ambient condition checks      │   │
│                           │  · Uncertainty limit validation  │   │
│                           │  · Certificate issuance         │   │
│                           └──────────────────────────────────┘  │
│                                    │                             │
│                    ┌───────────────▼───────────────┐            │
│                    │       PostgreSQL 16            │            │
│                    │                               │            │
│                    │  instruments                  │            │
│                    │  calibration_records          │            │
│                    │  measurements                 │            │
│                    │  certificates                 │            │
│                    │  calibration_events  ◀── append-only      │
│                    │  (event sourcing / audit log) │            │
│                    └───────────────────────────────┘            │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Web Dashboard  (/)  — dark theme, auto-refresh 15s      │   │
│  │  Stats · Instruments table · Records table · Live data   │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## Features

- **ISO 17025 compliance engine** — ambient temperature (18–28 °C) and humidity (30–75 %RH) checks; per-point uncertainty validation (|deviation| ≤ 2u)
- **Full calibration lifecycle** — draft → completed → certified → expired
- **Certificate generation** — sequential `CAL-YYYY-NNNNNN` numbering with configurable validity period
- **Append-only event store** — complete audit trail for every state transition
- **PostgreSQL persistence** — `pgx/v5` connection pooling, full foreign-key integrity
- **Real-time dashboard** — single-page dark-themed UI, auto-refreshes every 15 s
- **Docker Compose** — one-command local deployment with Postgres 16
- **Multi-stage Docker build** — distroless runtime image (~8 MB)
- **GitHub Actions CI** — race-condition tests, `go vet`, `golangci-lint`

---

## Quick Start

### Prerequisites

- Go 1.22+
- Docker & Docker Compose

### Run with Docker Compose

```bash
git clone https://github.com/aliipou/cloud-calibration.git
cd cloud-calibration

# Start Postgres + API
docker compose -f deployments/docker-compose.yml up --build

# Open dashboard
open http://localhost:8080
```

### Run locally (requires Postgres)

```bash
# Apply database schema
psql postgres://caluser:calpass@localhost:5432/calibration \
  -f internal/store/migrations/001_init.sql

# Start API
DATABASE_URL=postgres://caluser:calpass@localhost:5432/calibration \
  go run ./cmd/api
```

---

## API Reference

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/instruments` | Register a new instrument |
| `GET`  | `/api/v1/instruments` | List instruments (`?limit=&offset=`) |
| `GET`  | `/api/v1/instruments/:id` | Get instrument by ID |
| `POST` | `/api/v1/records` | Create calibration record |
| `GET`  | `/api/v1/records` | List records (`?instrument_id=&status=&limit=&offset=`) |
| `GET`  | `/api/v1/records/:id` | Get record with measurements |
| `POST` | `/api/v1/records/:id/complete` | Transition record to `completed` |
| `POST` | `/api/v1/records/:id/measurements` | Add a measurement point |
| `GET`  | `/api/v1/records/:id/compliance` | Run ISO 17025 compliance check |
| `POST` | `/api/v1/records/:id/certify` | Issue certificate (`{"validity_days":365}`) |
| `GET`  | `/api/v1/certificates/:record_id` | Retrieve certificate |
| `GET`  | `/api/v1/events/:aggregate_id` | Full audit trail for any aggregate |
| `GET`  | `/api/v1/stats` | Platform statistics |
| `GET`  | `/` | Web dashboard |

### Example: Full calibration workflow

```bash
# 1. Register instrument
curl -s -X POST http://localhost:8080/api/v1/instruments \
  -H 'Content-Type: application/json' \
  -d '{"serial_no":"SN-2026-001","model":"MC6-Ex","manufacturer":"Beamex","type":"pressure"}' | jq .

# 2. Create record
INST_ID="<id from step 1>"
curl -s -X POST http://localhost:8080/api/v1/records \
  -H 'Content-Type: application/json' \
  -d "{\"instrument_id\":\"$INST_ID\",\"technician\":\"ali.p\",\"temperature_c\":22.5,\"humidity_pct\":48.0,\"calibrated_at\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}" | jq .

# 3. Add measurement points
REC_ID="<id from step 2>"
curl -s -X POST http://localhost:8080/api/v1/records/$REC_ID/measurements \
  -H 'Content-Type: application/json' \
  -d '{"nominal":0.0,"actual":0.001,"uncertainty":0.005,"unit":"bar"}' | jq .

# 4. Complete & check compliance
curl -s -X POST http://localhost:8080/api/v1/records/$REC_ID/complete
curl -s http://localhost:8080/api/v1/records/$REC_ID/compliance | jq .

# 5. Issue certificate
curl -s -X POST http://localhost:8080/api/v1/records/$REC_ID/certify \
  -H 'Content-Type: application/json' \
  -d '{"validity_days":365}' | jq .
```

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://caluser:calpass@localhost:5432/calibration` | PostgreSQL connection string |
| `HTTP_PORT` | `8080` | API server listen port |

---

## Running Tests

```bash
# Unit tests (no database required)
go test ./internal/models/... ./internal/calibration/... -v

# All tests with race detector
go test ./... -race -count=1 -timeout 60s

# Coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.22 |
| HTTP framework | Gin v1.10 |
| Database driver | pgx/v5 (connection pool) |
| Database | PostgreSQL 16 |
| Logging | Uber zap |
| UUID | google/uuid |
| Container runtime | distroless/static-debian12 |
| CI | GitHub Actions |
| Lint | golangci-lint |

---

## Project Structure

```
cloud-calibration/
├── cmd/api/            # Entry point (main.go)
├── internal/
│   ├── api/            # HTTP handlers + route registration
│   ├── calibration/    # ISO 17025 compliance engine + service
│   ├── models/         # Domain types and DTOs
│   └── store/
│       ├── postgres.go          # pgx/v5 persistence layer
│       └── migrations/
│           └── 001_init.sql     # Database schema
├── web/
│   └── index.html      # Single-page dashboard
├── deployments/
│   └── docker-compose.yml
├── Dockerfile.api
└── .github/workflows/ci.yml
```
