package handler

import (
	"context"
	"net/http"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// UsageCache provides access to cached usage data.
type UsageCache interface {
	ListCopilotUsages(ctx context.Context) ([]domainquota.CopilotUsage, error)
}

// UsageCredentialRepository provides credential profile lookup for usage enrichment.
type UsageCredentialRepository interface {
	List(ctx context.Context) ([]domaincredential.Profile, error)
}

// SessionQuotaReader provides LLMPool session quota state for an account.
type SessionQuotaReader interface {
	GetUsage(ctx context.Context, providerType, accountID string) (*domainquota.SessionQuotaUsage, error)
}

// UsageHandler handles usage-related HTTP requests.
type UsageHandler struct {
	cache              UsageCache
	credentialRepo     UsageCredentialRepository
	sessionQuotaReader SessionQuotaReader
	logger             *zap.Logger
}

// NewUsageHandler creates a new usage handler.
func NewUsageHandler(
	cache UsageCache,
	credentialRepo UsageCredentialRepository,
	sessionQuotaReader SessionQuotaReader,
	logger *zap.Logger,
) *UsageHandler {
	return &UsageHandler{
		cache:              cache,
		credentialRepo:     credentialRepo,
		sessionQuotaReader: sessionQuotaReader,
		logger:             logger,
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

	if h.credentialRepo != nil && h.sessionQuotaReader != nil {
		profiles, err := h.credentialRepo.List(c.Request.Context())
		if err != nil {
			h.logger.Error("failed to list credential profiles for usage enrichment", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve usage data"})
			return
		}

		accountByCredential := make(map[string]string, len(profiles))
		for _, profile := range profiles {
			if profile.Type != "copilot" || profile.ID == "" || profile.AccountID == "" {
				continue
			}
			accountByCredential[profile.ID] = profile.AccountID
		}

		for i := range usages {
			accountID := accountByCredential[usages[i].CredentialID]
			if accountID == "" {
				continue
			}

			sessionQuota, readErr := h.sessionQuotaReader.GetUsage(c.Request.Context(), "copilot", accountID)
			if readErr != nil {
				h.logger.Warn("failed to enrich usage with session quota",
					zap.String("credential_id", usages[i].CredentialID),
					zap.String("account_id", accountID),
					zap.Error(readErr),
				)
				continue
			}
			usages[i].SessionQuota = sessionQuota
		}
	}

	c.JSON(http.StatusOK, UsageResponse{
		Usages: usages,
		Count:  len(usages),
	})
}
