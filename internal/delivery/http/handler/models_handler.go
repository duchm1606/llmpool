package handler

import (
	"errors"
	"net/http"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ModelsHandler handles model listing requests.
type ModelsHandler struct {
	service usecasecompletion.CompletionService
	logger  *zap.Logger
}

// NewModelsHandler creates a new models handler.
func NewModelsHandler(service usecasecompletion.CompletionService, logger *zap.Logger) *ModelsHandler {
	return &ModelsHandler{
		service: service,
		logger:  logger,
	}
}

// ListModels handles GET /v1/models
func (h *ModelsHandler) ListModels(c *gin.Context) {
	ctx := c.Request.Context()

	h.logger.Debug("listing models")

	resp, err := h.service.ListModels(ctx)
	if err != nil {
		var apiErr *domaincompletion.APIError
		if errors.As(err, &apiErr) {
			c.JSON(apiErr.HTTPStatus, domaincompletion.APIErrorResponse{Error: *apiErr})
			return
		}
		c.JSON(http.StatusInternalServerError, domaincompletion.APIErrorResponse{
			Error: *domaincompletion.ErrInternalServer(err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetModel handles GET /v1/models/:model
func (h *ModelsHandler) GetModel(c *gin.Context) {
	modelID := c.Param("model")
	ctx := c.Request.Context()

	h.logger.Debug("getting model", zap.String("model", modelID))

	// Get all models and find the requested one
	resp, err := h.service.ListModels(ctx)
	if err != nil {
		var apiErr *domaincompletion.APIError
		if errors.As(err, &apiErr) {
			c.JSON(apiErr.HTTPStatus, domaincompletion.APIErrorResponse{Error: *apiErr})
			return
		}
		c.JSON(http.StatusInternalServerError, domaincompletion.APIErrorResponse{
			Error: *domaincompletion.ErrInternalServer(err.Error()),
		})
		return
	}

	// Find the model
	for _, model := range resp.Data {
		if model.ID == modelID {
			c.JSON(http.StatusOK, model)
			return
		}
	}

	// Model not found
	apiErr := domaincompletion.ErrModelNotFound(modelID)
	c.JSON(apiErr.HTTPStatus, domaincompletion.APIErrorResponse{Error: *apiErr})
}
