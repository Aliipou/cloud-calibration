CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS instruments (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    serial_no    TEXT UNIQUE NOT NULL,
    model        TEXT NOT NULL,
    manufacturer TEXT NOT NULL,
    type         TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS calibration_records (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instrument_id UUID NOT NULL REFERENCES instruments(id),
    technician    TEXT NOT NULL,
    temperature_c FLOAT8 NOT NULL DEFAULT 20.0,
    humidity_pct  FLOAT8 NOT NULL DEFAULT 50.0,
    status        TEXT NOT NULL DEFAULT 'draft',
    calibrated_at TIMESTAMPTZ NOT NULL,
    due_date      TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS measurements (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    record_id   UUID NOT NULL REFERENCES calibration_records(id) ON DELETE CASCADE,
    nominal     FLOAT8 NOT NULL,
    actual      FLOAT8 NOT NULL,
    deviation   FLOAT8 NOT NULL,
    uncertainty FLOAT8 NOT NULL,
    unit        TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS certificates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    record_id   UUID NOT NULL REFERENCES calibration_records(id) UNIQUE,
    cert_number TEXT UNIQUE NOT NULL,
    issued_at   TIMESTAMPTZ NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    signature   TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Append-only event log (event sourcing)
CREATE TABLE IF NOT EXISTS calibration_events (
    id           BIGSERIAL PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    event_type   TEXT NOT NULL,
    payload      JSONB NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_calibration_events_aggregate ON calibration_events(aggregate_id);
CREATE INDEX IF NOT EXISTS idx_calibration_records_instrument ON calibration_records(instrument_id);
CREATE INDEX IF NOT EXISTS idx_calibration_records_status ON calibration_records(status);
