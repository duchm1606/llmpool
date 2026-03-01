package handler

import (
	"net/http"

	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
	"github.com/gin-gonic/gin"
)

type RefreshHandler struct {
	refreshService usecasecredential.RefreshService
}

func NewRefreshHandler(refreshService usecasecredential.RefreshService) *RefreshHandler {
	return &RefreshHandler{refreshService: refreshService}
}

func (h *RefreshHandler) Refresh(c *gin.Context) {
	if err := h.refreshService.RefreshDue(c.Request.Context()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "refresh triggered"})
}
