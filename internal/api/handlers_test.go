package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aliipou/cloud-calibration/internal/api"
	"github.com/aliipou/cloud-calibration/internal/calibration"
	"github.com/aliipou/cloud-calibration/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func init() { gin.SetMode(gin.TestMode) }

// ── fake store ───────────────────────────────────────────────────────────────

type fakeStore struct {
	instruments map[uuid.UUID]*models.Instrument
	records     map[uuid.UUID]*models.CalibrationRecord
	certs       map[uuid.UUID]*models.Certificate
	events      map[uuid.UUID][]*models.CalibrationEvent
	stats       map[string]any
	err         error // force all write ops to fail when non-nil
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		instruments: make(map[uuid.UUID]*models.Instrument),
		records:     make(map[uuid.UUID]*models.CalibrationRecord),
		certs:       make(map[uuid.UUID]*models.Certificate),
		events:      make(map[uuid.UUID][]*models.CalibrationEvent),
		stats:       map[string]any{"total_instruments": int64(0), "total_records": int64(0), "total_certified": int64(0), "total_measurements": int64(0)},
	}
}

func (f *fakeStore) CreateInstrument(_ context.Context, req *models.CreateInstrumentRequest) (*models.Instrument, error) {
	if f.err != nil {
		return nil, f.err
	}
	inst := &models.Instrument{ID: uuid.New(), SerialNo: req.SerialNo, Model: req.Model, Manufacturer: req.Manufacturer, Type: req.Type, CreatedAt: time.Now()}
	f.instruments[inst.ID] = inst
	return inst, nil
}

func (f *fakeStore) GetInstrument(_ context.Context, id uuid.UUID) (*models.Instrument, error) {
	inst, ok := f.instruments[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return inst, nil
}

func (f *fakeStore) ListInstruments(_ context.Context, limit, offset int) ([]*models.Instrument, int64, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	var list []*models.Instrument
	for _, v := range f.instruments {
		list = append(list, v)
	}
	total := int64(len(list))
	end := offset + limit
	if offset >= len(list) {
		return nil, total, nil
	}
	if end > len(list) {
		end = len(list)
	}
	return list[offset:end], total, nil
}

func (f *fakeStore) CreateRecord(_ context.Context, req *models.CreateRecordRequest) (*models.CalibrationRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	rec := &models.CalibrationRecord{
		ID: uuid.New(), InstrumentID: req.InstrumentID, Technician: req.Technician,
		TemperatureC: req.TemperatureC, HumidityPct: req.HumidityPct,
		Status: models.StatusDraft, CalibratedAt: req.CalibratedAt, DueDate: req.DueDate, CreatedAt: time.Now(),
	}
	f.records[rec.ID] = rec
	return rec, nil
}

func (f *fakeStore) GetRecord(_ context.Context, id uuid.UUID) (*models.CalibrationRecord, error) {
	rec, ok := f.records[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return rec, nil
}

func (f *fakeStore) ListRecords(_ context.Context, _ *uuid.UUID, _ *models.RecordStatus, limit, offset int) ([]*models.CalibrationRecord, int64, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	var list []*models.CalibrationRecord
	for _, v := range f.records {
		list = append(list, v)
	}
	return list, int64(len(list)), nil
}

func (f *fakeStore) AddMeasurement(_ context.Context, recordID uuid.UUID, req *models.AddMeasurementRequest) (*models.Measurement, error) {
	if f.err != nil {
		return nil, f.err
	}
	m := &models.Measurement{
		ID: uuid.New(), RecordID: recordID,
		Nominal: req.Nominal, Actual: req.Actual, Deviation: req.Actual - req.Nominal,
		Uncertainty: req.Uncertainty, Unit: req.Unit,
	}
	if rec, ok := f.records[recordID]; ok {
		rec.Measurements = append(rec.Measurements, *m)
	}
	return m, nil
}

func (f *fakeStore) CompleteRecord(_ context.Context, id uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	rec, ok := f.records[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	rec.Status = models.StatusCompleted
	return nil
}

func (f *fakeStore) CreateCertificate(_ context.Context, recordID uuid.UUID, expiresAt time.Time) (*models.Certificate, error) {
	if f.err != nil {
		return nil, f.err
	}
	cert := &models.Certificate{
		ID: uuid.New(), RecordID: recordID, CertNumber: fmt.Sprintf("CAL-%d-000001", time.Now().Year()),
		IssuedAt: time.Now(), ExpiresAt: expiresAt, CreatedAt: time.Now(),
	}
	f.certs[recordID] = cert
	return cert, nil
}

func (f *fakeStore) GetCertificate(_ context.Context, recordID uuid.UUID) (*models.Certificate, error) {
	cert, ok := f.certs[recordID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return cert, nil
}

func (f *fakeStore) GetEvents(_ context.Context, aggregateID uuid.UUID) ([]*models.CalibrationEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.events[aggregateID], nil
}

func (f *fakeStore) GetStats(_ context.Context) (map[string]any, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.stats, nil
}

// ── fakeSvcStore is a calibration.Store adapter for the service ──────────────

type fakeSvcStore struct{ *fakeStore }

func (fs *fakeSvcStore) GetRecord(ctx context.Context, id uuid.UUID) (*models.CalibrationRecord, error) {
	return fs.fakeStore.GetRecord(ctx, id)
}
func (fs *fakeSvcStore) CompleteRecord(ctx context.Context, id uuid.UUID) error {
	return fs.fakeStore.CompleteRecord(ctx, id)
}
func (fs *fakeSvcStore) CreateCertificate(ctx context.Context, recordID uuid.UUID, expiresAt time.Time) (*models.Certificate, error) {
	return fs.fakeStore.CreateCertificate(ctx, recordID, expiresAt)
}
func (fs *fakeSvcStore) GetCertificate(ctx context.Context, recordID uuid.UUID) (*models.Certificate, error) {
	return fs.fakeStore.GetCertificate(ctx, recordID)
}

// ── test router helper ───────────────────────────────────────────────────────

func newTestRouter(fs *fakeStore) *gin.Engine {
	svc := calibration.NewService(&fakeSvcStore{fs})
	h := api.NewHandler(fs, svc)
	r := gin.New()
	api.RegisterRoutes(r, h)
	return r
}

func post(t *testing.T, r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func get(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ── Instrument tests ─────────────────────────────────────────────────────────

func TestCreateInstrument_Success(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := post(t, r, "/api/v1/instruments", map[string]any{
		"serial_no": "SN-001", "model": "PM620", "manufacturer": "Beamex", "type": "pressure",
	})
	if w.Code != http.StatusCreated {
		t.Errorf("status=%d, want 201", w.Code)
	}
	var inst models.Instrument
	json.Unmarshal(w.Body.Bytes(), &inst)
	if inst.SerialNo != "SN-001" {
		t.Errorf("serial_no=%s, want SN-001", inst.SerialNo)
	}
}

func TestCreateInstrument_MissingFields(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := post(t, r, "/api/v1/instruments", map[string]any{"model": "PM620"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestCreateInstrument_StoreError(t *testing.T) {
	fs := newFakeStore()
	fs.err = fmt.Errorf("db down")
	r := newTestRouter(fs)
	w := post(t, r, "/api/v1/instruments", map[string]any{
		"serial_no": "SN-001", "model": "PM620", "manufacturer": "Beamex", "type": "pressure",
	})
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status=%d, want 500", w.Code)
	}
}

func TestListInstruments_Success(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := get(t, r, "/api/v1/instruments")
	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
}

func TestListInstruments_StoreError(t *testing.T) {
	fs := newFakeStore()
	fs.err = fmt.Errorf("db down")
	r := newTestRouter(fs)
	w := get(t, r, "/api/v1/instruments")
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status=%d, want 500", w.Code)
	}
}

func TestGetInstrument_Found(t *testing.T) {
	fs := newFakeStore()
	id := uuid.New()
	fs.instruments[id] = &models.Instrument{ID: id, SerialNo: "SN-X", Model: "TC305", Manufacturer: "Beamex", Type: models.InstrumentTemperature}
	r := newTestRouter(fs)
	w := get(t, r, "/api/v1/instruments/"+id.String())
	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
}

func TestGetInstrument_NotFound(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := get(t, r, "/api/v1/instruments/"+uuid.New().String())
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

func TestGetInstrument_InvalidUUID(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := get(t, r, "/api/v1/instruments/not-a-uuid")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// ── Record tests ─────────────────────────────────────────────────────────────

func TestCreateRecord_Success(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := post(t, r, "/api/v1/records", map[string]any{
		"instrument_id": uuid.New().String(),
		"technician":    "alice",
		"calibrated_at": time.Now().Format(time.RFC3339),
		"temperature_c": 22.0,
		"humidity_pct":  50.0,
	})
	if w.Code != http.StatusCreated {
		t.Errorf("status=%d, want 201 — body: %s", w.Code, w.Body.String())
	}
}

func TestCreateRecord_BadJSON(t *testing.T) {
	r := newTestRouter(newFakeStore())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/records", bytes.NewReader([]byte("{")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestCreateRecord_StoreError(t *testing.T) {
	fs := newFakeStore()
	fs.err = fmt.Errorf("db down")
	r := newTestRouter(fs)
	w := post(t, r, "/api/v1/records", map[string]any{
		"instrument_id": uuid.New().String(),
		"technician":    "alice",
		"calibrated_at": time.Now().Format(time.RFC3339),
	})
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status=%d, want 500", w.Code)
	}
}

func TestListRecords(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := get(t, r, "/api/v1/records")
	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
}

func TestListRecords_WithInvalidInstrumentID(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := get(t, r, "/api/v1/records?instrument_id=bad")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestGetRecord_Found(t *testing.T) {
	fs := newFakeStore()
	id := uuid.New()
	fs.records[id] = &models.CalibrationRecord{ID: id, Technician: "bob", Status: models.StatusDraft, CalibratedAt: time.Now()}
	r := newTestRouter(fs)
	w := get(t, r, "/api/v1/records/"+id.String())
	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
}

func TestGetRecord_NotFound(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := get(t, r, "/api/v1/records/"+uuid.New().String())
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

func TestCompleteRecord_Success(t *testing.T) {
	fs := newFakeStore()
	id := uuid.New()
	fs.records[id] = &models.CalibrationRecord{ID: id, Status: models.StatusDraft}
	r := newTestRouter(fs)
	w := post(t, r, "/api/v1/records/"+id.String()+"/complete", nil)
	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
}

func TestCompleteRecord_StoreError(t *testing.T) {
	fs := newFakeStore()
	id := uuid.New()
	fs.records[id] = &models.CalibrationRecord{ID: id, Status: models.StatusDraft}
	fs.err = fmt.Errorf("db error")
	r := newTestRouter(fs)
	w := post(t, r, "/api/v1/records/"+id.String()+"/complete", nil)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status=%d, want 500", w.Code)
	}
}

func TestAddMeasurement_Success(t *testing.T) {
	fs := newFakeStore()
	id := uuid.New()
	fs.records[id] = &models.CalibrationRecord{ID: id, Status: models.StatusDraft}
	r := newTestRouter(fs)
	w := post(t, r, "/api/v1/records/"+id.String()+"/measurements", map[string]any{
		"nominal": 100.0, "actual": 100.02, "uncertainty": 0.05, "unit": "Pa",
	})
	if w.Code != http.StatusCreated {
		t.Errorf("status=%d, want 201 — body: %s", w.Code, w.Body.String())
	}
}

func TestAddMeasurement_InvalidUUID(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := post(t, r, "/api/v1/records/bad-uuid/measurements", map[string]any{
		"nominal": 100.0, "actual": 100.02, "uncertainty": 0.05, "unit": "Pa",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestAddMeasurement_BadJSON(t *testing.T) {
	fs := newFakeStore()
	id := uuid.New()
	fs.records[id] = &models.CalibrationRecord{ID: id}
	r := newTestRouter(fs)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/records/"+id.String()+"/measurements", bytes.NewReader([]byte("{bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// ── Compliance tests ─────────────────────────────────────────────────────────

func TestCheckCompliance_NotFound(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := get(t, r, "/api/v1/records/"+uuid.New().String()+"/compliance")
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

func TestCheckCompliance_Compliant(t *testing.T) {
	fs := newFakeStore()
	id := uuid.New()
	fs.records[id] = &models.CalibrationRecord{
		ID: id, TemperatureC: 22.0, HumidityPct: 50.0, Status: models.StatusCompleted,
		Measurements: []models.Measurement{{Nominal: 100, Actual: 100.01, Deviation: 0.01, Uncertainty: 0.05, Unit: "Pa"}},
	}
	r := newTestRouter(fs)
	w := get(t, r, "/api/v1/records/"+id.String()+"/compliance")
	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
}

// ── Certify tests ─────────────────────────────────────────────────────────────

func TestCertify_Success(t *testing.T) {
	fs := newFakeStore()
	id := uuid.New()
	fs.records[id] = &models.CalibrationRecord{
		ID: id, TemperatureC: 22.0, HumidityPct: 50.0, Status: models.StatusCompleted,
		Measurements: []models.Measurement{{Nominal: 100, Actual: 100.01, Deviation: 0.01, Uncertainty: 0.05, Unit: "Pa"}},
	}
	r := newTestRouter(fs)
	w := post(t, r, "/api/v1/records/"+id.String()+"/certify", map[string]any{"validity_days": 365})
	if w.Code != http.StatusCreated {
		t.Errorf("status=%d, want 201 — body: %s", w.Code, w.Body.String())
	}
}

func TestCertify_NonCompliant(t *testing.T) {
	fs := newFakeStore()
	id := uuid.New()
	fs.records[id] = &models.CalibrationRecord{
		ID: id, TemperatureC: 22.0, HumidityPct: 50.0, Status: models.StatusCompleted,
		Measurements: []models.Measurement{{Nominal: 100, Actual: 200, Deviation: 100, Uncertainty: 0.05, Unit: "Pa"}},
	}
	r := newTestRouter(fs)
	w := post(t, r, "/api/v1/records/"+id.String()+"/certify", map[string]any{"validity_days": 365})
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status=%d, want 422", w.Code)
	}
}

func TestCertify_InvalidUUID(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := post(t, r, "/api/v1/records/bad/certify", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// ── Certificate tests ────────────────────────────────────────────────────────

func TestGetCertificate_NotFound(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := get(t, r, "/api/v1/certificates/"+uuid.New().String())
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

func TestGetCertificate_Found(t *testing.T) {
	fs := newFakeStore()
	recordID := uuid.New()
	fs.certs[recordID] = &models.Certificate{
		ID: uuid.New(), RecordID: recordID, CertNumber: "CAL-2026-000001",
		IssuedAt: time.Now(), ExpiresAt: time.Now().AddDate(1, 0, 0), CreatedAt: time.Now(),
	}
	r := newTestRouter(fs)
	w := get(t, r, "/api/v1/certificates/"+recordID.String())
	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
}

func TestGetCertificate_InvalidUUID(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := get(t, r, "/api/v1/certificates/not-a-uuid")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// ── Events / audit trail tests ───────────────────────────────────────────────

func TestGetEvents_Success(t *testing.T) {
	fs := newFakeStore()
	aggID := uuid.New()
	fs.events[aggID] = []*models.CalibrationEvent{
		{ID: 1, AggregateID: aggID, EventType: "instrument.created", CreatedAt: time.Now()},
	}
	r := newTestRouter(fs)
	w := get(t, r, "/api/v1/events/"+aggID.String())
	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
}

func TestGetEvents_InvalidUUID(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := get(t, r, "/api/v1/events/not-a-uuid")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestGetEvents_StoreError(t *testing.T) {
	fs := newFakeStore()
	fs.err = fmt.Errorf("db down")
	r := newTestRouter(fs)
	w := get(t, r, "/api/v1/events/"+uuid.New().String())
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status=%d, want 500", w.Code)
	}
}

// ── Stats tests ──────────────────────────────────────────────────────────────

func TestGetStats_Success(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := get(t, r, "/api/v1/stats")
	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
}

func TestGetStats_StoreError(t *testing.T) {
	fs := newFakeStore()
	fs.err = fmt.Errorf("db down")
	r := newTestRouter(fs)
	w := get(t, r, "/api/v1/stats")
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status=%d, want 500", w.Code)
	}
}

// ── CORS middleware test ─────────────────────────────────────────────────────

func TestCORSHeaders(t *testing.T) {
	r := newTestRouter(newFakeStore())
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/instruments", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("missing CORS header, got: %s", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

// ── Pagination helper ────────────────────────────────────────────────────────

func TestPagination_Defaults(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := get(t, r, "/api/v1/instruments")
	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["limit"] != float64(20) {
		t.Errorf("default limit=%v, want 20", resp["limit"])
	}
}

func TestPagination_CustomValues(t *testing.T) {
	r := newTestRouter(newFakeStore())
	w := get(t, r, "/api/v1/instruments?limit=5&offset=10")
	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["limit"] != float64(5) {
		t.Errorf("limit=%v, want 5", resp["limit"])
	}
	if resp["offset"] != float64(10) {
		t.Errorf("offset=%v, want 10", resp["offset"])
	}
}
