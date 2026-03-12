package calibration_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aliipou/cloud-calibration/internal/calibration"
	"github.com/aliipou/cloud-calibration/internal/models"
	"github.com/google/uuid"
)

// fakeStore is an in-memory implementation of calibration.Store for tests.
type fakeStore struct {
	records      map[uuid.UUID]*models.CalibrationRecord
	certificates map[uuid.UUID]*models.Certificate
	completeErr  error
	certErr      error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		records:      make(map[uuid.UUID]*models.CalibrationRecord),
		certificates: make(map[uuid.UUID]*models.Certificate),
	}
}

func (f *fakeStore) GetRecord(_ context.Context, id uuid.UUID) (*models.CalibrationRecord, error) {
	rec, ok := f.records[id]
	if !ok {
		return nil, errors.New("record not found")
	}
	return rec, nil
}

func (f *fakeStore) CompleteRecord(_ context.Context, id uuid.UUID) error {
	if f.completeErr != nil {
		return f.completeErr
	}
	rec, ok := f.records[id]
	if !ok {
		return errors.New("record not found")
	}
	rec.Status = models.StatusCompleted
	return nil
}

func (f *fakeStore) CreateCertificate(_ context.Context, recordID uuid.UUID, expiresAt time.Time) (*models.Certificate, error) {
	if f.certErr != nil {
		return nil, f.certErr
	}
	cert := &models.Certificate{
		ID:         uuid.New(),
		RecordID:   recordID,
		CertNumber: "CAL-2026-000001",
		IssuedAt:   time.Now().UTC(),
		ExpiresAt:  expiresAt,
		CreatedAt:  time.Now().UTC(),
	}
	f.certificates[recordID] = cert
	if rec, ok := f.records[recordID]; ok {
		rec.Status = models.StatusCertified
	}
	return cert, nil
}

func (f *fakeStore) GetCertificate(_ context.Context, recordID uuid.UUID) (*models.Certificate, error) {
	cert, ok := f.certificates[recordID]
	if !ok {
		return nil, errors.New("certificate not found")
	}
	return cert, nil
}

// makeRecord creates a calibration record with sane defaults.
func makeRecord(status models.RecordStatus, tempC, humPct float64, measurements []models.Measurement) *models.CalibrationRecord {
	return &models.CalibrationRecord{
		ID:           uuid.New(),
		InstrumentID: uuid.New(),
		Technician:   "test-tech",
		TemperatureC: tempC,
		HumidityPct:  humPct,
		Status:       status,
		CalibratedAt: time.Now().UTC(),
		Measurements: measurements,
		CreatedAt:    time.Now().UTC(),
	}
}

func goodMeasurements() []models.Measurement {
	return []models.Measurement{
		{ID: uuid.New(), RecordID: uuid.New(), Nominal: 10.0, Actual: 10.01, Deviation: 0.01, Uncertainty: 0.05, Unit: "bar"},
		{ID: uuid.New(), RecordID: uuid.New(), Nominal: 20.0, Actual: 20.02, Deviation: 0.02, Uncertainty: 0.05, Unit: "bar"},
	}
}

func TestCheckCompliance_NoMeasurements(t *testing.T) {
	store := newFakeStore()
	svc := calibration.NewService(store)

	rec := makeRecord(models.StatusDraft, 22.0, 50.0, nil)
	store.records[rec.ID] = rec

	result, err := svc.CheckCompliance(context.Background(), rec.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Compliant {
		t.Error("expected non-compliant result with no measurements")
	}

	found := false
	for _, v := range result.Violations {
		if strings.Contains(v, "no measurements") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected violation about 'no measurements', got: %v", result.Violations)
	}
}

func TestCheckCompliance_OutOfRangeTemperature(t *testing.T) {
	store := newFakeStore()
	svc := calibration.NewService(store)

	rec := makeRecord(models.StatusDraft, 35.0, 50.0, goodMeasurements())
	store.records[rec.ID] = rec

	result, err := svc.CheckCompliance(context.Background(), rec.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Compliant {
		t.Error("expected non-compliant result with out-of-range temperature")
	}

	found := false
	for _, v := range result.Violations {
		if strings.Contains(v, "temperature") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected temperature violation, got: %v", result.Violations)
	}
}

func TestCheckCompliance_OutOfRangeHumidity(t *testing.T) {
	store := newFakeStore()
	svc := calibration.NewService(store)

	rec := makeRecord(models.StatusDraft, 22.0, 20.0, goodMeasurements())
	store.records[rec.ID] = rec

	result, err := svc.CheckCompliance(context.Background(), rec.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Compliant {
		t.Error("expected non-compliant result with out-of-range humidity")
	}

	found := false
	for _, v := range result.Violations {
		if strings.Contains(v, "humidity") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected humidity violation, got: %v", result.Violations)
	}
}

func TestCheckCompliance_ExceedsUncertainty(t *testing.T) {
	store := newFakeStore()
	svc := calibration.NewService(store)

	measurements := []models.Measurement{
		// |deviation| = 0.15 > 2 * 0.05 = 0.10 → violation
		{ID: uuid.New(), RecordID: uuid.New(), Nominal: 10.0, Actual: 10.15, Deviation: 0.15, Uncertainty: 0.05, Unit: "bar"},
	}
	rec := makeRecord(models.StatusDraft, 22.0, 50.0, measurements)
	store.records[rec.ID] = rec

	result, err := svc.CheckCompliance(context.Background(), rec.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Compliant {
		t.Error("expected non-compliant result when deviation exceeds 2*uncertainty")
	}

	found := false
	for _, v := range result.Violations {
		if strings.Contains(v, "uncertainty limit") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected uncertainty violation, got: %v", result.Violations)
	}
}

func TestCheckCompliance_AllGood(t *testing.T) {
	store := newFakeStore()
	svc := calibration.NewService(store)

	rec := makeRecord(models.StatusCompleted, 22.0, 50.0, goodMeasurements())
	store.records[rec.ID] = rec

	result, err := svc.CheckCompliance(context.Background(), rec.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Compliant {
		t.Errorf("expected compliant result, got violations: %v", result.Violations)
	}
	if len(result.Violations) != 0 {
		t.Errorf("expected zero violations, got %d: %v", len(result.Violations), result.Violations)
	}
}

func TestCertify_RequiresCompliance(t *testing.T) {
	store := newFakeStore()
	svc := calibration.NewService(store)

	// Record with no measurements → compliance fails
	rec := makeRecord(models.StatusCompleted, 22.0, 50.0, nil)
	store.records[rec.ID] = rec

	_, err := svc.Certify(context.Background(), rec.ID, 365)
	if err == nil {
		t.Error("expected error when certifying a non-compliant record")
	}
	if !strings.Contains(err.Error(), "not compliant") {
		t.Errorf("expected 'not compliant' in error, got: %v", err)
	}
}

func TestCertify_RequiresCompleted(t *testing.T) {
	store := newFakeStore()
	svc := calibration.NewService(store)

	// Draft record with valid measurements
	rec := makeRecord(models.StatusDraft, 22.0, 50.0, goodMeasurements())
	store.records[rec.ID] = rec

	_, err := svc.Certify(context.Background(), rec.ID, 365)
	if err == nil {
		t.Error("expected error when certifying a draft record")
	}
	if !strings.Contains(err.Error(), "draft") {
		t.Errorf("expected 'draft' in error, got: %v", err)
	}
}

func TestMaxDeviation(t *testing.T) {
	svc := calibration.NewService(newFakeStore())

	tests := []struct {
		name     string
		record   *models.CalibrationRecord
		wantMax  float64
		tol      float64
	}{
		{
			name:    "no measurements",
			record:  makeRecord(models.StatusDraft, 22.0, 50.0, nil),
			wantMax: 0.0,
			tol:     1e-9,
		},
		{
			name: "single measurement",
			record: makeRecord(models.StatusDraft, 22.0, 50.0, []models.Measurement{
				{Deviation: -0.05},
			}),
			wantMax: 0.05,
			tol:     1e-9,
		},
		{
			name: "multiple measurements picks max absolute",
			record: makeRecord(models.StatusDraft, 22.0, 50.0, []models.Measurement{
				{Deviation: 0.01},
				{Deviation: -0.08},
				{Deviation: 0.03},
			}),
			wantMax: 0.08,
			tol:     1e-9,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.MaxDeviation(tc.record)
			diff := got - tc.wantMax
			if diff < 0 {
				diff = -diff
			}
			if diff > tc.tol {
				t.Errorf("MaxDeviation = %.10f, want %.10f", got, tc.wantMax)
			}
		})
	}
}

func TestAverageDeviation(t *testing.T) {
	svc := calibration.NewService(newFakeStore())

	tests := []struct {
		name    string
		record  *models.CalibrationRecord
		wantAvg float64
		tol     float64
	}{
		{
			name:    "no measurements",
			record:  makeRecord(models.StatusDraft, 22.0, 50.0, nil),
			wantAvg: 0.0,
			tol:     1e-9,
		},
		{
			name: "two measurements",
			record: makeRecord(models.StatusDraft, 22.0, 50.0, []models.Measurement{
				{Deviation: 0.04},
				{Deviation: -0.02},
			}),
			// (0.04 + 0.02) / 2 = 0.03
			wantAvg: 0.03,
			tol:     1e-9,
		},
		{
			name: "three measurements",
			record: makeRecord(models.StatusDraft, 22.0, 50.0, []models.Measurement{
				{Deviation: 0.06},
				{Deviation: -0.03},
				{Deviation: 0.03},
			}),
			// (0.06 + 0.03 + 0.03) / 3 = 0.04
			wantAvg: 0.04,
			tol:     1e-9,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.AverageDeviation(tc.record)
			diff := got - tc.wantAvg
			if diff < 0 {
				diff = -diff
			}
			if diff > tc.tol {
				t.Errorf("AverageDeviation = %.10f, want %.10f", got, tc.wantAvg)
			}
		})
	}
}
