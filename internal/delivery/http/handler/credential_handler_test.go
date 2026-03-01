package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
	"github.com/gin-gonic/gin"
)

type importServiceStub struct {
	profile domaincredential.Profile
	err     error
}

func (s importServiceStub) Import(_ context.Context, _ usecasecredential.ImportInput) (domaincredential.Profile, error) {
	if s.err != nil {
		return domaincredential.Profile{}, s.err
	}
	return s.profile, nil
}

func TestCredentialHandler_Import_ValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewCredentialHandler(importServiceStub{})
	r := gin.New()
	r.POST("/v1/internal/auth-profiles/import", h.Import)

	reqBody := `{"provider":"openai","payload":{"email":"user@example.com"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/internal/auth-profiles/import", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if body["message"] != "validation failed" {
		t.Fatalf("expected validation failed message, got %v", body["message"])
	}

	errors, ok := body["errors"].([]any)
	if !ok || len(errors) == 0 {
		t.Fatalf("expected non-empty validation errors")
	}
}

func TestCredentialHandler_Import_Created(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Now().UTC()
	stub := importServiceStub{profile: domaincredential.Profile{
		ID:              "p1",
		Provider:        "openai",
		Label:           "openai-profile",
		Email:           "user@example.com",
		AccountID:       "acc-1",
		Status:          "active",
		HasRefreshToken: true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}}

	h := NewCredentialHandler(stub)
	r := gin.New()
	r.POST("/v1/internal/auth-profiles/import", h.Import)

	reqBody := `{"provider":"openai","payload":{"access_token":"token","refresh_token":"refresh","email":"user@example.com","account_id":"acc-1"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/internal/auth-profiles/import", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}
}
