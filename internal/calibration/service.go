package calibration

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/aliipou/cloud-calibration/internal/models"
	"github.com/google/uuid"
)

// Store defines the persistence interface used by the calibration service.
type Store interface {
	GetRecord(ctx context.Context, id uuid.UUID) (*models.CalibrationRecord, error)
	CompleteRecord(ctx context.Context, id uuid.UUID) error
	CreateCertificate(ctx context.Context, recordID uuid.UUID, expiresAt time.Time) (*models.Certificate, error)
	GetCertificate(ctx context.Context, recordID uuid.UUID) (*models.Certificate, error)
}

// Service implements ISO 17025 calibration business rules.
type Service struct {
	store Store
}

// NewService creates a calibration Service backed by the given store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// CheckCompliance validates a calibration record against ISO 17025 requirements.
// It checks ambient conditions and measurement uncertainty limits.
func (s *Service) CheckCompliance(ctx context.Context, recordID uuid.UUID) (*models.ComplianceResult, error) {
	rec, err := s.store.GetRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("CheckCompliance: %w", err)
	}

	var violations []string

	// ISO 17025: at least one measurement point is required
	if len(rec.Measurements) == 0 {
		violations = append(violations, "no measurements recorded")
	}

	// ISO/IEC 17025 §6.4: controlled ambient conditions
	// Typical requirement: temperature 18–28 °C
	if rec.TemperatureC < 18.0 || rec.TemperatureC > 28.0 {
		violations = append(violations, fmt.Sprintf(
			"ambient temperature out of range (18–28 °C): got %.1f °C", rec.TemperatureC,
		))
	}

	// Typical requirement: relative humidity 30–75 %RH
	if rec.HumidityPct < 30.0 || rec.HumidityPct > 75.0 {
		violations = append(violations, fmt.Sprintf(
			"ambient humidity out of range (30–75 %%RH): got %.1f %%RH", rec.HumidityPct,
		))
	}

	// Each measurement: |deviation| must not exceed 2 × expanded uncertainty (k=2)
	for _, m := range rec.Measurements {
		absDev := math.Abs(m.Deviation)
		limit := m.Uncertainty * 2.0
		if absDev > limit {
			violations = append(violations, fmt.Sprintf(
				"measurement point %.4f %s exceeds uncertainty limit (|dev|=%.4f > 2u=%.4f)",
				m.Nominal, m.Unit, absDev, limit,
			))
		}
	}

	return &models.ComplianceResult{
		RecordID:   recordID,
		Compliant:  len(violations) == 0,
		Violations: violations,
		CheckedAt:  time.Now().UTC(),
	}, nil
}

// Certify issues an ISO 17025 calibration certificate for a completed record.
// The record must pass compliance checks and have status "completed".
func (s *Service) Certify(ctx context.Context, recordID uuid.UUID, validityDays int) (*models.Certificate, error) {
	result, err := s.CheckCompliance(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("Certify compliance check: %w", err)
	}
	if !result.Compliant {
		return nil, fmt.Errorf("record %s is not compliant: %v", recordID, result.Violations)
	}

	rec, err := s.store.GetRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("Certify get record: %w", err)
	}

	if rec.Status == models.StatusDraft {
		return nil, fmt.Errorf("record %s must be completed before certification (current status: draft)", recordID)
	}
	if rec.Status == models.StatusCertified {
		// Idempotent: return existing certificate
		return s.store.GetCertificate(ctx, recordID)
	}

	expiresAt := time.Now().UTC().AddDate(0, 0, validityDays)
	cert, err := s.store.CreateCertificate(ctx, recordID, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("Certify create certificate: %w", err)
	}
	return cert, nil
}

// MaxDeviation returns the maximum absolute deviation across all measurements in the record.
// Returns 0 if no measurements exist.
func (s *Service) MaxDeviation(record *models.CalibrationRecord) float64 {
	if len(record.Measurements) == 0 {
		return 0
	}
	max := 0.0
	for _, m := range record.Measurements {
		absDev := math.Abs(m.Deviation)
		if absDev > max {
			max = absDev
		}
	}
	return max
}

// AverageDeviation returns the mean absolute deviation across all measurements.
// Returns 0 if no measurements exist.
func (s *Service) AverageDeviation(record *models.CalibrationRecord) float64 {
	if len(record.Measurements) == 0 {
		return 0
	}
	sum := 0.0
	for _, m := range record.Measurements {
		sum += math.Abs(m.Deviation)
	}
	return sum / float64(len(record.Measurements))
}
