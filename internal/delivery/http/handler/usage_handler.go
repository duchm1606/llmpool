package handler

import (
	"context"
	"net/http"

	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// UsageCache provides access to cached usage data.
type UsageCache interface {
	ListCopilotUsages(ctx context.Context) ([]domainquota.CopilotUsage, error)
}

// UsageHandler handles usage-related HTTP requests.
type UsageHandler struct {
	cache  UsageCache
	logger *zap.Logger
}

// NewUsageHandler creates a new usage handler.
func NewUsageHandler(cache UsageCache, logger *zap.Logger) *UsageHandler {
	return &UsageHandler{
		cache:  cache,
		logger: logger,
	}
}

// UsageResponse represents the response for the usage endpoint.
type UsageResponse struct {
	Usages []domainquota.CopilotUsage `json:"usages"`
	Count  int                        `json:"count"`
}

// ListUsages returns all cached Copilot usages.
// GET /v1/internal/usage
func (h *UsageHandler) ListUsages(c *gin.Context) {
	usages, err := h.cache.ListCopilotUsages(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list copilot usages", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve usage data"})
		return
	}

	c.JSON(http.StatusOK, UsageResponse{
		Usages: usages,
		Count:  len(usages),
	})
}
