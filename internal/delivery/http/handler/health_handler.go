package handler

import (
	"net/http"

	usecasehealth "github.com/duchoang/llmpool/internal/usecase/health"
	"github.com/gin-gonic/gin"
)

type HealthHandler struct {
	service usecasehealth.Service
}

func NewHealthHandler(service usecasehealth.Service) *HealthHandler {
	return &HealthHandler{service: service}
}

func (h *HealthHandler) Get(c *gin.Context) {
	status := h.service.GetStatus()
	c.JSON(http.StatusOK, status)
}
