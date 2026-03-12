# Roadmap

This document outlines the planned development phases for the Cloud Calibration Platform.

---

## Phase 1 — Foundation (Complete)

Core platform delivering ISO 17025 compliance management.

- [x] Append-only event store for full calibration audit trail
- [x] ISO 17025 compliance checks: ambient temperature (18–28 °C), relative humidity (30–75 %RH), per-point uncertainty limits (|deviation| ≤ 2u, k=2)
- [x] Complete calibration lifecycle: draft → completed → certified → expired
- [x] Sequential certificate generation (`CAL-YYYY-NNNNNN`)
- [x] REST API: instruments, records, measurements, compliance, certificates, audit trail, stats
- [x] Dark-themed single-page dashboard with live stats and 15 s auto-refresh
- [x] PostgreSQL 16 persistence with `pgx/v5` connection pooling
- [x] Multi-stage Docker build (distroless runtime, ~8 MB image)
- [x] GitHub Actions CI: race-condition tests, `go vet`, `golangci-lint`

---

## Phase 2 — PKI Signing & PDF Certificates

Make certificates tamper-evident and machine-verifiable.

- [ ] **RSA-2048 signing** — sign a SHA-256 hash of `cert_number + record_id + issued_at + expires_at` using PKCS#1 v1.5; store base64-encoded signature in `certificates.signature`
- [ ] **Verification endpoint** — `GET /api/v1/certificates/:id/verify` re-computes hash and validates signature against the stored public key
- [ ] **PDF generation** — use `github.com/jung-kurt/gofpdf` to produce a formatted ISO 17025 certificate PDF with calibration table, uncertainty budget, ambient conditions, and technician details
- [ ] **QR code** — embed a QR code in the PDF linking to the verification endpoint (`github.com/skip2/go-qrcode`)
- [ ] **Streaming download** — `GET /api/v1/certificates/:id/pdf` streams the PDF with `Content-Disposition: attachment`

---

## Phase 3 — Time-Series Measurements with TimescaleDB

Enable long-term trend analysis and 10-year data retention.

- [ ] **TimescaleDB hypertable** — migrate `measurements` table to a hypertable partitioned by `created_at`
- [ ] **Continuous aggregates** — daily and monthly deviation trend views per instrument type
- [ ] **10-year retention policy** — `add_retention_policy` aligned with ISO 17025 record-keeping requirements
- [ ] **Trend API** — `GET /api/v1/instruments/:id/trends?from=&to=&interval=1d` returns aggregated deviation statistics
- [ ] **Dashboard charts** — line charts for deviation over time using the trend API

---

## Phase 4 — Multi-Tenant & Authentication

Isolate labs and enforce role-based access control.

- [ ] **Lab (tenant) isolation** — add `lab_id` column to instruments, records, and certificates; enforce row-level security in PostgreSQL
- [ ] **OIDC/JWT authentication** — validate Azure Entra ID JWTs (`github.com/lestrrat-go/jwx`) on all API routes
- [ ] **RBAC** — three roles:
  - `technician` — create instruments, records, and measurements; run compliance checks
  - `reviewer` — approve and complete records; view all records within lab
  - `admin` — issue certificates; manage lab users; access stats across all labs
- [ ] **Federated identity** — support Entra ID app registration with PKCE flow for dashboard login
- [ ] **Audit events for auth** — emit `auth.login`, `auth.token_refresh` events to the event store

---

## Phase 5 — Event Bus & Worker Scaling

Decouple PDF generation and notifications from the API hot path.

- [ ] **Kafka producer** — publish every `CalibrationEvent` to the `calibration-events` Kafka topic (Sarama client)
- [ ] **PDF worker** — Kafka consumer group that generates and stores PDFs in Azure Blob Storage on `record.certified` events
- [ ] **KEDA scaling** — `ScaledObject` targeting the `calibration-events` consumer group lag; scale PDF worker pods 0–20
- [ ] **Webhook notifications** — configurable outbound webhooks triggered on `record.certified` and `record.expired` events; delivery with exponential back-off retry
- [ ] **Dead-letter topic** — failed PDF jobs routed to `calibration-events-dlq` with alerting via Azure Monitor action group

---

## Phase 6 — AKS Production Deployment

Harden the platform for enterprise Azure deployment.

- [ ] **Helm chart** — parameterised chart for `api`, `worker`, `postgres` (or Azure Database for PostgreSQL Flexible Server), and KEDA `ScaledObject`
- [ ] **Azure Key Vault** — store RSA private key in Key Vault; retrieve via Workload Identity (no static secrets); rotate key annually
- [ ] **Defender for Cloud** — enable Defender for Containers; remediate all HIGH/CRITICAL findings before each release; export security score to Log Analytics
- [ ] **Automated compliance reporting** — weekly Azure Logic App that queries `/api/v1/stats`, formats an ISO 17025 summary report, and distributes via email / Teams
- [ ] **DR runbook** — Azure Automation runbook for point-in-time restore of PostgreSQL to secondary region (RTO < 4 h, RPO < 1 h)
- [ ] **Private networking** — AKS with Azure CNI Overlay; API and DB exposed only on private endpoints; Front Door + WAF (Prevention mode) as public ingress
