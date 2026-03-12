package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/aliipou/cloud-calibration/internal/calibration"
	"github.com/aliipou/cloud-calibration/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CalibrationStore is the persistence interface required by the API handlers.
type CalibrationStore interface {
	CreateInstrument(ctx context.Context, req *models.CreateInstrumentRequest) (*models.Instrument, error)
	GetInstrument(ctx context.Context, id uuid.UUID) (*models.Instrument, error)
	ListInstruments(ctx context.Context, limit, offset int) ([]*models.Instrument, int64, error)
	CreateRecord(ctx context.Context, req *models.CreateRecordRequest) (*models.CalibrationRecord, error)
	GetRecord(ctx context.Context, id uuid.UUID) (*models.CalibrationRecord, error)
	ListRecords(ctx context.Context, instrumentID *uuid.UUID, status *models.RecordStatus, limit, offset int) ([]*models.CalibrationRecord, int64, error)
	AddMeasurement(ctx context.Context, recordID uuid.UUID, req *models.AddMeasurementRequest) (*models.Measurement, error)
	CompleteRecord(ctx context.Context, id uuid.UUID) error
	CreateCertificate(ctx context.Context, recordID uuid.UUID, expiresAt time.Time) (*models.Certificate, error)
	GetCertificate(ctx context.Context, recordID uuid.UUID) (*models.Certificate, error)
	GetEvents(ctx context.Context, aggregateID uuid.UUID) ([]*models.CalibrationEvent, error)
	GetStats(ctx context.Context) (map[string]any, error)
}

// Handler bundles all HTTP handler dependencies.
type Handler struct {
	store   CalibrationStore
	service *calibration.Service
}

// NewHandler creates a Handler with the provided store and service.
func NewHandler(s CalibrationStore, svc *calibration.Service) *Handler {
	return &Handler{store: s, service: svc}
}

// RegisterRoutes attaches all API routes to the given Gin engine.
func RegisterRoutes(r *gin.Engine, h *Handler) {
	// Serve dashboard
	r.StaticFile("/", "./web/index.html")

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	v1 := r.Group("/api/v1")
	{
		// Instruments
		v1.POST("/instruments", h.CreateInstrument)
		v1.GET("/instruments", h.ListInstruments)
		v1.GET("/instruments/:id", h.GetInstrument)

		// Calibration records
		v1.POST("/records", h.CreateRecord)
		v1.GET("/records", h.ListRecords)
		v1.GET("/records/:id", h.GetRecord)
		v1.POST("/records/:id/complete", h.CompleteRecord)
		v1.POST("/records/:id/measurements", h.AddMeasurement)
		v1.GET("/records/:id/compliance", h.CheckCompliance)
		v1.POST("/records/:id/certify", h.Certify)

		// Certificates
		v1.GET("/certificates/:record_id", h.GetCertificate)

		// Audit trail
		v1.GET("/events/:aggregate_id", h.GetEvents)

		// Stats
		v1.GET("/stats", h.GetStats)
	}
}

// --- Instrument handlers ---

func (h *Handler) CreateInstrument(c *gin.Context) {
	var req models.CreateInstrumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	inst, err := h.store.CreateInstrument(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, inst)
}

func (h *Handler) ListInstruments(c *gin.Context) {
	limit, offset := pagination(c)

	instruments, total, err := h.store.ListInstruments(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":   instruments,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) GetInstrument(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	inst, err := h.store.GetInstrument(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, inst)
}

// --- Record handlers ---

func (h *Handler) CreateRecord(c *gin.Context) {
	var req models.CreateRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rec, err := h.store.CreateRecord(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, rec)
}

func (h *Handler) ListRecords(c *gin.Context) {
	limit, offset := pagination(c)

	var instrumentID *uuid.UUID
	if idStr := c.Query("instrument_id"); idStr != "" {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instrument_id"})
			return
		}
		instrumentID = &id
	}

	var status *models.RecordStatus
	if s := c.Query("status"); s != "" {
		rs := models.RecordStatus(s)
		status = &rs
	}

	records, total, err := h.store.ListRecords(c.Request.Context(), instrumentID, status, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":   records,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) GetRecord(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	rec, err := h.store.GetRecord(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, rec)
}

func (h *Handler) CompleteRecord(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	if err := h.store.CompleteRecord(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "completed", "id": id})
}

func (h *Handler) AddMeasurement(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	var req models.AddMeasurementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	m, err := h.store.AddMeasurement(c.Request.Context(), id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, m)
}

func (h *Handler) CheckCompliance(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	result, err := h.service.CheckCompliance(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Certify(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	var body struct {
		ValidityDays int `json:"validity_days"`
	}
	body.ValidityDays = 365 // default
	if err := c.ShouldBindJSON(&body); err != nil {
		// body is optional; use default
	}
	if body.ValidityDays <= 0 {
		body.ValidityDays = 365
	}

	cert, err := h.service.Certify(c.Request.Context(), id, body.ValidityDays)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, cert)
}

// --- Certificate handlers ---

func (h *Handler) GetCertificate(c *gin.Context) {
	recordID, err := parseUUID(c, "record_id")
	if err != nil {
		return
	}

	cert, err := h.store.GetCertificate(c.Request.Context(), recordID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, cert)
}

// --- Audit trail ---

func (h *Handler) GetEvents(c *gin.Context) {
	aggregateID, err := parseUUID(c, "aggregate_id")
	if err != nil {
		return
	}

	events, err := h.store.GetEvents(c.Request.Context(), aggregateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": events, "total": len(events)})
}

// --- Stats ---

func (h *Handler) GetStats(c *gin.Context) {
	stats, err := h.store.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Calculate compliance rate
	total, _ := stats["total_records"].(int64)
	certified, _ := stats["total_certified"].(int64)
	rate := 0.0
	if total > 0 {
		rate = float64(certified) / float64(total) * 100.0
	}
	stats["compliance_rate"] = rate

	c.JSON(http.StatusOK, stats)
}

// --- Helpers ---

func parseUUID(c *gin.Context, param string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + param})
		return uuid.Nil, err
	}
	return id, nil
}

func pagination(c *gin.Context) (limit, offset int) {
	limit = 20
	offset = 0
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 200 {
			limit = v
		}
	}
	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}
	return limit, offset
}

