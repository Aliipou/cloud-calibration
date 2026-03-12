package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aliipou/cloud-calibration/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides access to the PostgreSQL database.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a new Store and verifies the connection.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases all pool connections.
func (s *Store) Close() { s.pool.Close() }

// CreateInstrument inserts a new instrument record and emits an event.
func (s *Store) CreateInstrument(ctx context.Context, req *models.CreateInstrumentRequest) (*models.Instrument, error) {
	const q = `
		INSERT INTO instruments (serial_no, model, manufacturer, type)
		VALUES ($1, $2, $3, $4)
		RETURNING id, serial_no, model, manufacturer, type, created_at`

	var inst models.Instrument
	row := s.pool.QueryRow(ctx, q, req.SerialNo, req.Model, req.Manufacturer, string(req.Type))
	if err := row.Scan(&inst.ID, &inst.SerialNo, &inst.Model, &inst.Manufacturer, &inst.Type, &inst.CreatedAt); err != nil {
		return nil, fmt.Errorf("CreateInstrument scan: %w", err)
	}

	if err := s.AppendEvent(ctx, inst.ID, "instrument.created", inst); err != nil {
		return nil, fmt.Errorf("CreateInstrument event: %w", err)
	}

	return &inst, nil
}

// GetInstrument retrieves an instrument by ID.
func (s *Store) GetInstrument(ctx context.Context, id uuid.UUID) (*models.Instrument, error) {
	const q = `
		SELECT id, serial_no, model, manufacturer, type, created_at
		FROM instruments WHERE id = $1`

	var inst models.Instrument
	row := s.pool.QueryRow(ctx, q, id)
	if err := row.Scan(&inst.ID, &inst.SerialNo, &inst.Model, &inst.Manufacturer, &inst.Type, &inst.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("instrument not found: %s", id)
		}
		return nil, fmt.Errorf("GetInstrument scan: %w", err)
	}
	return &inst, nil
}

// ListInstruments returns a paginated list of instruments with total count.
func (s *Store) ListInstruments(ctx context.Context, limit, offset int) ([]*models.Instrument, int64, error) {
	const countQ = `SELECT COUNT(*) FROM instruments`
	var total int64
	if err := s.pool.QueryRow(ctx, countQ).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListInstruments count: %w", err)
	}

	const q = `
		SELECT id, serial_no, model, manufacturer, type, created_at
		FROM instruments
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`

	rows, err := s.pool.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("ListInstruments query: %w", err)
	}
	defer rows.Close()

	var instruments []*models.Instrument
	for rows.Next() {
		var inst models.Instrument
		if err := rows.Scan(&inst.ID, &inst.SerialNo, &inst.Model, &inst.Manufacturer, &inst.Type, &inst.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("ListInstruments scan: %w", err)
		}
		instruments = append(instruments, &inst)
	}
	return instruments, total, rows.Err()
}

// CreateRecord inserts a new calibration record and emits an event.
func (s *Store) CreateRecord(ctx context.Context, req *models.CreateRecordRequest) (*models.CalibrationRecord, error) {
	const q = `
		INSERT INTO calibration_records (instrument_id, technician, temperature_c, humidity_pct, calibrated_at, due_date)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, instrument_id, technician, temperature_c, humidity_pct, status, calibrated_at, due_date, created_at`

	var rec models.CalibrationRecord
	row := s.pool.QueryRow(ctx, q,
		req.InstrumentID,
		req.Technician,
		req.TemperatureC,
		req.HumidityPct,
		req.CalibratedAt,
		req.DueDate,
	)
	if err := row.Scan(
		&rec.ID, &rec.InstrumentID, &rec.Technician,
		&rec.TemperatureC, &rec.HumidityPct, &rec.Status,
		&rec.CalibratedAt, &rec.DueDate, &rec.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("CreateRecord scan: %w", err)
	}

	if err := s.AppendEvent(ctx, rec.ID, "record.created", rec); err != nil {
		return nil, fmt.Errorf("CreateRecord event: %w", err)
	}

	return &rec, nil
}

// GetRecord retrieves a calibration record by ID and loads its measurements.
func (s *Store) GetRecord(ctx context.Context, id uuid.UUID) (*models.CalibrationRecord, error) {
	const q = `
		SELECT id, instrument_id, technician, temperature_c, humidity_pct, status, calibrated_at, due_date, created_at
		FROM calibration_records WHERE id = $1`

	var rec models.CalibrationRecord
	row := s.pool.QueryRow(ctx, q, id)
	if err := row.Scan(
		&rec.ID, &rec.InstrumentID, &rec.Technician,
		&rec.TemperatureC, &rec.HumidityPct, &rec.Status,
		&rec.CalibratedAt, &rec.DueDate, &rec.CreatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("record not found: %s", id)
		}
		return nil, fmt.Errorf("GetRecord scan: %w", err)
	}

	measurements, err := s.getMeasurements(ctx, id)
	if err != nil {
		return nil, err
	}
	rec.Measurements = measurements

	return &rec, nil
}

func (s *Store) getMeasurements(ctx context.Context, recordID uuid.UUID) ([]models.Measurement, error) {
	const q = `
		SELECT id, record_id, nominal, actual, deviation, uncertainty, unit
		FROM measurements WHERE record_id = $1
		ORDER BY nominal ASC`

	rows, err := s.pool.Query(ctx, q, recordID)
	if err != nil {
		return nil, fmt.Errorf("getMeasurements query: %w", err)
	}
	defer rows.Close()

	var measurements []models.Measurement
	for rows.Next() {
		var m models.Measurement
		if err := rows.Scan(&m.ID, &m.RecordID, &m.Nominal, &m.Actual, &m.Deviation, &m.Uncertainty, &m.Unit); err != nil {
			return nil, fmt.Errorf("getMeasurements scan: %w", err)
		}
		measurements = append(measurements, m)
	}
	return measurements, rows.Err()
}

// ListRecords returns paginated calibration records with optional filters.
func (s *Store) ListRecords(ctx context.Context, instrumentID *uuid.UUID, status *models.RecordStatus, limit, offset int) ([]*models.CalibrationRecord, int64, error) {
	args := []any{}
	argIdx := 1
	where := ""

	if instrumentID != nil {
		where += fmt.Sprintf(" WHERE instrument_id = $%d", argIdx)
		args = append(args, *instrumentID)
		argIdx++
	}
	if status != nil {
		if where == "" {
			where += fmt.Sprintf(" WHERE status = $%d", argIdx)
		} else {
			where += fmt.Sprintf(" AND status = $%d", argIdx)
		}
		args = append(args, string(*status))
		argIdx++
	}

	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM calibration_records%s`, where)
	var total int64
	if err := s.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListRecords count: %w", err)
	}

	q := fmt.Sprintf(`
		SELECT id, instrument_id, technician, temperature_c, humidity_pct, status, calibrated_at, due_date, created_at
		FROM calibration_records%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("ListRecords query: %w", err)
	}
	defer rows.Close()

	var records []*models.CalibrationRecord
	for rows.Next() {
		var rec models.CalibrationRecord
		if err := rows.Scan(
			&rec.ID, &rec.InstrumentID, &rec.Technician,
			&rec.TemperatureC, &rec.HumidityPct, &rec.Status,
			&rec.CalibratedAt, &rec.DueDate, &rec.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("ListRecords scan: %w", err)
		}
		records = append(records, &rec)
	}
	return records, total, rows.Err()
}

// AddMeasurement inserts a measurement with computed deviation.
func (s *Store) AddMeasurement(ctx context.Context, recordID uuid.UUID, req *models.AddMeasurementRequest) (*models.Measurement, error) {
	deviation := req.Actual - req.Nominal

	const q = `
		INSERT INTO measurements (record_id, nominal, actual, deviation, uncertainty, unit)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, record_id, nominal, actual, deviation, uncertainty, unit`

	var m models.Measurement
	row := s.pool.QueryRow(ctx, q, recordID, req.Nominal, req.Actual, deviation, req.Uncertainty, req.Unit)
	if err := row.Scan(&m.ID, &m.RecordID, &m.Nominal, &m.Actual, &m.Deviation, &m.Uncertainty, &m.Unit); err != nil {
		return nil, fmt.Errorf("AddMeasurement scan: %w", err)
	}
	return &m, nil
}

// CompleteRecord transitions a record to "completed" status.
func (s *Store) CompleteRecord(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE calibration_records SET status = 'completed' WHERE id = $1`
	ct, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("CompleteRecord exec: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("record not found: %s", id)
	}

	if err := s.AppendEvent(ctx, id, "record.completed", map[string]any{"record_id": id, "status": "completed"}); err != nil {
		return fmt.Errorf("CompleteRecord event: %w", err)
	}
	return nil
}

// CreateCertificate issues a calibration certificate for a completed record.
func (s *Store) CreateCertificate(ctx context.Context, recordID uuid.UUID, expiresAt time.Time) (*models.Certificate, error) {
	// Generate sequential certificate number: CAL-YYYY-NNNNNN
	year := time.Now().UTC().Year()
	var seq int64
	seqQ := `SELECT COALESCE(MAX(CAST(SPLIT_PART(cert_number, '-', 3) AS BIGINT)), 0) + 1 FROM certificates`
	if err := s.pool.QueryRow(ctx, seqQ).Scan(&seq); err != nil {
		return nil, fmt.Errorf("CreateCertificate seq: %w", err)
	}
	certNumber := fmt.Sprintf("CAL-%d-%06d", year, seq)

	now := time.Now().UTC()
	const q = `
		INSERT INTO certificates (record_id, cert_number, issued_at, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, record_id, cert_number, issued_at, expires_at, COALESCE(signature, ''), created_at`

	var cert models.Certificate
	row := s.pool.QueryRow(ctx, q, recordID, certNumber, now, expiresAt)
	if err := row.Scan(&cert.ID, &cert.RecordID, &cert.CertNumber, &cert.IssuedAt, &cert.ExpiresAt, &cert.Signature, &cert.CreatedAt); err != nil {
		return nil, fmt.Errorf("CreateCertificate scan: %w", err)
	}

	// Promote record to certified status
	const updateQ = `UPDATE calibration_records SET status = 'certified' WHERE id = $1`
	if _, err := s.pool.Exec(ctx, updateQ, recordID); err != nil {
		return nil, fmt.Errorf("CreateCertificate update record: %w", err)
	}

	if err := s.AppendEvent(ctx, recordID, "record.certified", map[string]any{
		"record_id":   recordID,
		"cert_number": certNumber,
		"issued_at":   now,
		"expires_at":  expiresAt,
	}); err != nil {
		return nil, fmt.Errorf("CreateCertificate event: %w", err)
	}

	return &cert, nil
}

// GetCertificate retrieves a certificate by its associated record ID.
func (s *Store) GetCertificate(ctx context.Context, recordID uuid.UUID) (*models.Certificate, error) {
	const q = `
		SELECT id, record_id, cert_number, issued_at, expires_at, COALESCE(signature, ''), created_at
		FROM certificates WHERE record_id = $1`

	var cert models.Certificate
	row := s.pool.QueryRow(ctx, q, recordID)
	if err := row.Scan(&cert.ID, &cert.RecordID, &cert.CertNumber, &cert.IssuedAt, &cert.ExpiresAt, &cert.Signature, &cert.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("certificate not found for record: %s", recordID)
		}
		return nil, fmt.Errorf("GetCertificate scan: %w", err)
	}
	return &cert, nil
}

// AppendEvent writes an event to the append-only calibration_events table.
func (s *Store) AppendEvent(ctx context.Context, aggregateID uuid.UUID, eventType string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("AppendEvent marshal: %w", err)
	}

	const q = `
		INSERT INTO calibration_events (aggregate_id, event_type, payload)
		VALUES ($1, $2, $3)`
	if _, err := s.pool.Exec(ctx, q, aggregateID, eventType, data); err != nil {
		return fmt.Errorf("AppendEvent exec: %w", err)
	}
	return nil
}

// GetEvents returns all events for an aggregate in chronological order.
func (s *Store) GetEvents(ctx context.Context, aggregateID uuid.UUID) ([]*models.CalibrationEvent, error) {
	const q = `
		SELECT id, aggregate_id, event_type, payload, created_at
		FROM calibration_events
		WHERE aggregate_id = $1
		ORDER BY id ASC`

	rows, err := s.pool.Query(ctx, q, aggregateID)
	if err != nil {
		return nil, fmt.Errorf("GetEvents query: %w", err)
	}
	defer rows.Close()

	var events []*models.CalibrationEvent
	for rows.Next() {
		var ev models.CalibrationEvent
		if err := rows.Scan(&ev.ID, &ev.AggregateID, &ev.EventType, &ev.Payload, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("GetEvents scan: %w", err)
		}
		events = append(events, &ev)
	}
	return events, rows.Err()
}

// GetStats returns aggregate counts for the platform dashboard.
func (s *Store) GetStats(ctx context.Context) (map[string]any, error) {
	stats := make(map[string]any)

	queries := []struct {
		key   string
		query string
	}{
		{"total_instruments", `SELECT COUNT(*) FROM instruments`},
		{"total_records", `SELECT COUNT(*) FROM calibration_records`},
		{"total_certified", `SELECT COUNT(*) FROM calibration_records WHERE status = 'certified'`},
		{"total_measurements", `SELECT COUNT(*) FROM measurements`},
	}

	for _, q := range queries {
		var count int64
		if err := s.pool.QueryRow(ctx, q.query).Scan(&count); err != nil {
			return nil, fmt.Errorf("GetStats %s: %w", q.key, err)
		}
		stats[q.key] = count
	}

	return stats, nil
}
