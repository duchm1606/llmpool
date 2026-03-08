package handler

import (
	"net/http"
	"strconv"
	"time"

	usecaseusage "github.com/duchoang/llmpool/internal/usecase/usage"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// UsageStatsHandler handles usage statistics endpoints.
type UsageStatsHandler struct {
	statsService     usecaseusage.StatsService
	retentionService usecaseusage.RetentionService
	logger           *zap.Logger
}

// NewUsageStatsHandler creates a new usage stats handler.
func NewUsageStatsHandler(
	statsService usecaseusage.StatsService,
	retentionService usecaseusage.RetentionService,
	logger *zap.Logger,
) *UsageStatsHandler {
	return &UsageStatsHandler{
		statsService:     statsService,
		retentionService: retentionService,
		logger:           logger,
	}
}

// GetDashboardStats handles GET /v1/internal/usage/stats
func (h *UsageStatsHandler) GetDashboardStats(c *gin.Context) {
	period := c.DefaultQuery("period", "today")

	// Validate period
	validPeriods := map[string]bool{"today": true, "7d": true, "30d": true, "90d": true, "365d": true}
	if !validPeriods[period] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid period, must be one of: today, 7d, 30d, 90d, 365d",
		})
		return
	}

	stats, err := h.statsService.GetDashboardStats(c.Request.Context(), period)
	if err != nil {
		h.logger.Error("failed to get dashboard stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// ListAuditLogs handles GET /v1/internal/usage/audit
func (h *UsageStatsHandler) ListAuditLogs(c *gin.Context) {
	// Parse query parameters
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	model := c.Query("model")
	provider := c.Query("provider")
	credentialID := c.Query("credential_id")
	status := c.Query("status")
	if status == "success" {
		status = "done"
	}
	if status == "error" {
		status = "failed"
	}
	validStatuses := map[string]bool{"": true, "done": true, "failed": true, "canceled": true}
	if !validStatuses[status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status, must be one of: done, failed, canceled"})
		return
	}

	// Parse time range
	var startTime, endTime time.Time
	var err error

	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")

	if startTimeStr != "" {
		startTime, err = time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_time format, use RFC3339"})
			return
		}
	} else {
		// Default to 24 hours ago
		startTime = time.Now().UTC().Add(-24 * time.Hour)
	}

	if endTimeStr != "" {
		endTime, err = time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end_time format, use RFC3339"})
			return
		}
	} else {
		endTime = time.Now().UTC()
	}

	filter := usecaseusage.AuditLogFilter{
		StartTime:    startTime,
		EndTime:      endTime,
		Limit:        limit,
		Offset:       offset,
		Model:        model,
		Provider:     provider,
		CredentialID: credentialID,
		Status:       status,
	}

	logs, total, err := h.statsService.GetAuditLogs(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to get audit logs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get audit logs"})
		return
	}

	page := (offset / limit) + 1
	totalPages := 0
	if limit > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        logs,
		"entries":     logs,
		"total":       total,
		"limit":       limit,
		"offset":      offset,
		"page":        page,
		"page_size":   limit,
		"total_pages": totalPages,
	})
}

// GetAuditLogByRequestID handles GET /v1/internal/usage/audit/:request_id
func (h *UsageStatsHandler) GetAuditLogByRequestID(c *gin.Context) {
	requestID := c.Param("request_id")
	if requestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request_id is required"})
		return
	}

	log, err := h.statsService.GetAuditLogByRequestID(c.Request.Context(), requestID)
	if err != nil {
		h.logger.Error("failed to get audit log", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get audit log"})
		return
	}

	if log == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audit log not found"})
		return
	}

	c.JSON(http.StatusOK, log)
}

// RunRetentionCleanup handles POST /v1/internal/usage/cleanup
func (h *UsageStatsHandler) RunRetentionCleanup(c *gin.Context) {
	deleted, err := h.retentionService.Cleanup(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to run retention cleanup", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to run cleanup"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "retention cleanup complete",
		"deleted_count": deleted,
	})
}
