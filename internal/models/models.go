package models

import (
	"time"

	"github.com/google/uuid"
)

// InstrumentType categorises the device under calibration.
type InstrumentType string

const (
	InstrumentPressure    InstrumentType = "pressure"
	InstrumentTemperature InstrumentType = "temperature"
	InstrumentFlow        InstrumentType = "flow"
	InstrumentHumidity    InstrumentType = "humidity"
	InstrumentElectrical  InstrumentType = "electrical"
)

// RecordStatus tracks the lifecycle of a calibration record.
type RecordStatus string

const (
	StatusDraft     RecordStatus = "draft"
	StatusCompleted RecordStatus = "completed"
	StatusCertified RecordStatus = "certified"
	StatusExpired   RecordStatus = "expired"
)

// Instrument represents a device to be calibrated.
type Instrument struct {
	ID           uuid.UUID      `json:"id"`
	SerialNo     string         `json:"serial_no"`
	Model        string         `json:"model"`
	Manufacturer string         `json:"manufacturer"`
	Type         InstrumentType `json:"type"`
	CreatedAt    time.Time      `json:"created_at"`
}

// Measurement is one calibration point: nominal vs. actual.
type Measurement struct {
	ID          uuid.UUID `json:"id"`
	RecordID    uuid.UUID `json:"record_id"`
	Nominal     float64   `json:"nominal"`      // reference / expected value
	Actual      float64   `json:"actual"`       // measured value
	Deviation   float64   `json:"deviation"`    // actual - nominal
	Uncertainty float64   `json:"uncertainty"`  // expanded uncertainty (k=2)
	Unit        string    `json:"unit"`
}

// CalibrationRecord captures a full calibration session.
type CalibrationRecord struct {
	ID           uuid.UUID     `json:"id"`
	InstrumentID uuid.UUID     `json:"instrument_id"`
	Technician   string        `json:"technician"`
	TemperatureC float64       `json:"temperature_c"`      // ambient °C
	HumidityPct  float64       `json:"humidity_pct"`       // ambient %RH
	Status       RecordStatus  `json:"status"`
	CalibratedAt time.Time     `json:"calibrated_at"`
	DueDate      *time.Time    `json:"due_date,omitempty"` // next calibration due
	Measurements []Measurement `json:"measurements,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
}

// Certificate is the ISO 17025 calibration certificate.
type Certificate struct {
	ID         uuid.UUID `json:"id"`
	RecordID   uuid.UUID `json:"record_id"`
	CertNumber string    `json:"cert_number"` // e.g. CAL-2026-000001
	IssuedAt   time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Signature  string    `json:"signature,omitempty"` // base64 PKI signature (placeholder)
	CreatedAt  time.Time `json:"created_at"`
}

// CalibrationEvent is one entry in the append-only event store.
type CalibrationEvent struct {
	ID          int64     `json:"id"`
	AggregateID uuid.UUID `json:"aggregate_id"` // instrument or record ID
	EventType   string    `json:"event_type"`
	Payload     []byte    `json:"payload"` // JSON
	CreatedAt   time.Time `json:"created_at"`
}

// --- request / response DTOs ---

type CreateInstrumentRequest struct {
	SerialNo     string         `json:"serial_no"     binding:"required"`
	Model        string         `json:"model"         binding:"required"`
	Manufacturer string         `json:"manufacturer"  binding:"required"`
	Type         InstrumentType `json:"type"          binding:"required"`
}

type CreateRecordRequest struct {
	InstrumentID uuid.UUID  `json:"instrument_id"  binding:"required"`
	Technician   string     `json:"technician"     binding:"required"`
	TemperatureC float64    `json:"temperature_c"`
	HumidityPct  float64    `json:"humidity_pct"`
	CalibratedAt time.Time  `json:"calibrated_at"  binding:"required"`
	DueDate      *time.Time `json:"due_date"`
}

type AddMeasurementRequest struct {
	Nominal     float64 `json:"nominal"     binding:"required"`
	Actual      float64 `json:"actual"      binding:"required"`
	Uncertainty float64 `json:"uncertainty" binding:"required"`
	Unit        string  `json:"unit"        binding:"required"`
}

type ComplianceResult struct {
	RecordID   uuid.UUID `json:"record_id"`
	Compliant  bool      `json:"compliant"`
	Violations []string  `json:"violations"`
	CheckedAt  time.Time `json:"checked_at"`
}
