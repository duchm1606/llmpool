package handler

import (
	"net/http"

	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
	"github.com/gin-gonic/gin"
)

type CredentialHandler struct {
	importService usecasecredential.ImportService
}

func NewCredentialHandler(importService usecasecredential.ImportService) *CredentialHandler {
	return &CredentialHandler{importService: importService}
}

type importCredentialRequest struct {
	Provider string                              `json:"provider"`
	Label    string                              `json:"label"`
	Source   string                              `json:"source"`
	Payload  usecasecredential.CredentialPayload `json:"payload" binding:"required"`
}

func (h *CredentialHandler) Import(c *gin.Context) {
	var req importCredentialRequest
	if !bindJSONAndValidate(c, &req) {
		return
	}

	profile, err := h.importService.Import(c.Request.Context(), usecasecredential.ImportInput{
		ProviderHint: req.Provider,
		Label:        req.Label,
		Source:       req.Source,
		Payload:      req.Payload,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":              profile.ID,
		"provider":        profile.Provider,
		"label":           profile.Label,
		"email":           profile.Email,
		"accountId":       profile.AccountID,
		"status":          profile.Status,
		"hasRefreshToken": profile.HasRefreshToken,
	})
}
