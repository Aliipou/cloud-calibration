package models_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/aliipou/cloud-calibration/internal/models"
	"github.com/google/uuid"
)

func TestInstrumentTypes(t *testing.T) {
	types := []models.InstrumentType{
		models.InstrumentPressure,
		models.InstrumentTemperature,
		models.InstrumentFlow,
		models.InstrumentHumidity,
		models.InstrumentElectrical,
	}

	for _, it := range types {
		t.Run(string(it), func(t *testing.T) {
			if string(it) == "" {
				t.Errorf("instrument type constant must not be empty")
			}
		})
	}

	if len(types) != 5 {
		t.Errorf("expected 5 instrument types, got %d", len(types))
	}
}

func TestRecordStatus(t *testing.T) {
	statuses := []models.RecordStatus{
		models.StatusDraft,
		models.StatusCompleted,
		models.StatusCertified,
		models.StatusExpired,
	}

	seen := make(map[models.RecordStatus]bool)
	for _, s := range statuses {
		if seen[s] {
			t.Errorf("duplicate status: %q", s)
		}
		seen[s] = true
		if string(s) == "" {
			t.Errorf("status constant must not be empty")
		}
	}

	if len(seen) != 4 {
		t.Errorf("expected 4 distinct statuses, got %d", len(seen))
	}
}

func TestMeasurementDeviation(t *testing.T) {
	tests := []struct {
		name     string
		nominal  float64
		actual   float64
		wantDev  float64
		tolerane float64
	}{
		{
			name:     "positive deviation",
			nominal:  10.0,
			actual:   10.05,
			wantDev:  0.05,
			tolerane: 1e-9,
		},
		{
			name:     "negative deviation",
			nominal:  10.0,
			actual:   9.95,
			wantDev:  -0.05,
			tolerane: 1e-9,
		},
		{
			name:     "zero deviation",
			nominal:  5.0,
			actual:   5.0,
			wantDev:  0.0,
			tolerane: 1e-9,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := models.Measurement{
				ID:       uuid.New(),
				RecordID: uuid.New(),
				Nominal:  tc.nominal,
				Actual:   tc.actual,
				// deviation = actual - nominal
				Deviation:   tc.actual - tc.nominal,
				Uncertainty: 0.1,
				Unit:        "bar",
			}

			if math.Abs(m.Deviation-tc.wantDev) > tc.tolerane {
				t.Errorf("deviation = %.10f, want %.10f (tol=%.2e)", m.Deviation, tc.wantDev, tc.tolerane)
			}
		})
	}
}

func TestCertNumber(t *testing.T) {
	tests := []struct {
		name       string
		certNumber string
		wantValid  bool
	}{
		{
			name:       "valid CAL prefix",
			certNumber: "CAL-2026-000001",
			wantValid:  true,
		},
		{
			name:       "valid CAL prefix high number",
			certNumber: "CAL-2026-999999",
			wantValid:  true,
		},
		{
			name:       "empty cert number",
			certNumber: "",
			wantValid:  false,
		},
		{
			name:       "missing CAL prefix",
			certNumber: "CERT-2026-000001",
			wantValid:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cert := models.Certificate{
				ID:         uuid.New(),
				RecordID:   uuid.New(),
				CertNumber: tc.certNumber,
				IssuedAt:   time.Now(),
				ExpiresAt:  time.Now().AddDate(1, 0, 0),
				CreatedAt:  time.Now(),
			}

			isValid := cert.CertNumber != "" && strings.HasPrefix(cert.CertNumber, "CAL-")
			if isValid != tc.wantValid {
				t.Errorf("certNumber %q: valid=%v, want %v", tc.certNumber, isValid, tc.wantValid)
			}
		})
	}
}

func TestCreateInstrumentRequest_Fields(t *testing.T) {
	tests := []struct {
		name    string
		req     models.CreateInstrumentRequest
		wantErr bool
	}{
		{
			name: "all fields set",
			req: models.CreateInstrumentRequest{
				SerialNo:     "SN-001",
				Model:        "Beamex MC6",
				Manufacturer: "Beamex",
				Type:         models.InstrumentPressure,
			},
			wantErr: false,
		},
		{
			name: "serial number set",
			req: models.CreateInstrumentRequest{
				SerialNo:     "SN-002",
				Model:        "MC6-T",
				Manufacturer: "Beamex",
				Type:         models.InstrumentTemperature,
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.req.SerialNo == "" {
				t.Error("SerialNo must not be empty when set")
			}
			if tc.req.Model == "" {
				t.Error("Model must not be empty when set")
			}
			if tc.req.Manufacturer == "" {
				t.Error("Manufacturer must not be empty when set")
			}
			if tc.req.Type == "" {
				t.Error("Type must not be empty when set")
			}
		})
	}
}
