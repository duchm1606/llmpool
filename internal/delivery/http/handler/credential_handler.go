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

func (h *CredentialHandler) Import(c *gin.Context) {
	var req usecasecredential.CredentialProfile
	if !bindJSONAndValidate(c, &req) {
		return
	}

	profile, err := h.importService.Import(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":                    profile.ID,
		"type":                  profile.Type,
		"email":                 profile.Email,
		"accountId":             profile.AccountID,
		"enabled":               profile.Enabled,
		"expired":               profile.Expired,
		"last_refresh":          profile.LastRefreshAt,
		"has_encrypted_profile": profile.EncryptedProfile != "",
	})
}
