package handler

import (
	"context"
	"encoding/json"
	"errors"
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

type listServiceStub struct {
	profiles []domaincredential.Profile
	err      error
}

type statusServiceStub struct {
	updated domaincredential.Profile
	err     error
	enabled *bool
	id      string
}

func (s importServiceStub) Import(_ context.Context, _ usecasecredential.CredentialProfile) (domaincredential.Profile, error) {
	if s.err != nil {
		return domaincredential.Profile{}, s.err
	}
	return s.profile, nil
}

func (s listServiceStub) List(_ context.Context) ([]domaincredential.Profile, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.profiles, nil
}

func (s *statusServiceStub) SetEnabled(_ context.Context, credentialID string, enabled bool) (domaincredential.Profile, error) {
	s.id = credentialID
	s.enabled = &enabled
	if s.err != nil {
		return domaincredential.Profile{}, s.err
	}

	updated := s.updated
	if updated.ID == "" {
		updated.ID = credentialID
	}
	updated.Enabled = enabled
	return updated, nil
}

func TestCredentialHandler_Import_ValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewCredentialHandler(importServiceStub{}, listServiceStub{}, &statusServiceStub{})
	r := gin.New()
	r.POST("/v1/internal/auth-profiles/import", h.Import)

	reqBody := `{"type":"openai","email":"user@example.com"}`
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

	now := time.Now()
	stub := importServiceStub{profile: domaincredential.Profile{
		ID:               "p1",
		Type:             "openai",
		Email:            "user@example.com",
		AccountID:        "acc-1",
		Enabled:          true,
		Expired:          now,
		LastRefreshAt:    now,
		EncryptedProfile: "enc:payload",
	}}

	h := NewCredentialHandler(stub, listServiceStub{}, &statusServiceStub{})
	r := gin.New()
	r.POST("/v1/internal/auth-profiles/import", h.Import)

	reqBody := `{"type":"openai","access_token":"token","refresh_token":"refresh","email":"user@example.com","account_id":"acc-1","enabled":true,"expired":"2027-01-01T00:00:00Z","last_refresh":"2027-01-01T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/internal/auth-profiles/import", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}
}

func TestCredentialHandler_List_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Now().UTC()
	h := NewCredentialHandler(importServiceStub{}, listServiceStub{
		profiles: []domaincredential.Profile{
			{
				ID:            "cred-1",
				Type:          "copilot",
				AccountID:     "octocat",
				Email:         "octocat@example.com",
				Enabled:       true,
				Expired:       now.Add(30 * time.Minute),
				LastRefreshAt: now,
			},
		},
	}, &statusServiceStub{})
	r := gin.New()
	r.GET("/v1/internal/auth-profiles", h.List)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/auth-profiles", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	count, ok := body["count"].(float64)
	if !ok || int(count) != 1 {
		t.Fatalf("expected count 1, got %#v", body["count"])
	}

	data, ok := body["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("expected one data row, got %#v", body["data"])
	}

	row, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected row payload: %#v", data[0])
	}

	if row["account_id"] != "octocat" {
		t.Fatalf("expected account_id octocat, got %#v", row["account_id"])
	}
	if row["type"] != "copilot" {
		t.Fatalf("expected type copilot, got %#v", row["type"])
	}
}

func TestCredentialHandler_List_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewCredentialHandler(importServiceStub{}, listServiceStub{err: errors.New("db down")}, &statusServiceStub{})
	r := gin.New()
	r.GET("/v1/internal/auth-profiles", h.List)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/auth-profiles", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if body["message"] != "failed to list credential profiles" {
		t.Fatalf("unexpected message: %#v", body["message"])
	}
}

func TestCredentialHandler_SetStatus_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Now().UTC()
	statusStub := &statusServiceStub{updated: domaincredential.Profile{LastRefreshAt: now, Expired: now.Add(time.Hour)}}
	h := NewCredentialHandler(importServiceStub{}, listServiceStub{}, statusStub)
	r := gin.New()
	r.PATCH("/v1/internal/auth-profiles/:id", h.SetStatus)

	req := httptest.NewRequest(http.MethodPatch, "/v1/internal/auth-profiles/cred-1", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if statusStub.id != "cred-1" {
		t.Fatalf("credential id = %q, want cred-1", statusStub.id)
	}
	if statusStub.enabled == nil || *statusStub.enabled != false {
		t.Fatalf("enabled flag not passed correctly")
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["enabled"] != false {
		t.Fatalf("enabled response = %#v, want false", body["enabled"])
	}
}

func TestCredentialHandler_SetStatus_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	statusStub := &statusServiceStub{err: usecasecredential.ErrCredentialNotFound}
	h := NewCredentialHandler(importServiceStub{}, listServiceStub{}, statusStub)
	r := gin.New()
	r.PATCH("/v1/internal/auth-profiles/:id", h.SetStatus)

	req := httptest.NewRequest(http.MethodPatch, "/v1/internal/auth-profiles/missing", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

func TestCredentialHandler_SetStatus_BadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewCredentialHandler(importServiceStub{}, listServiceStub{}, &statusServiceStub{})
	r := gin.New()
	r.PATCH("/v1/internal/auth-profiles/:id", h.SetStatus)

	req := httptest.NewRequest(http.MethodPatch, "/v1/internal/auth-profiles/cred-1", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}
