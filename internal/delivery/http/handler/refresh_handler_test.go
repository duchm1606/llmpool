package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	domainquota "github.com/duchoang/llmpool/internal/domain/quota"

	"github.com/gin-gonic/gin"
)

type refreshServiceStub struct {
	err              error
	lastCredentialID string
}

func (s refreshServiceStub) RefreshDue(_ context.Context) error {
	return s.err
}

func (s *refreshServiceStub) RefreshCredential(_ context.Context, credentialID string) error {
	s.lastCredentialID = credentialID
	return s.err
}

type quotaServiceStub struct {
	err            error
	checkedAll     bool
	lastCredential string
	state          *domainquota.CredentialState
}

func (s *quotaServiceStub) CheckSample(_ context.Context) error { return s.err }
func (s *quotaServiceStub) CheckAll(_ context.Context) error {
	s.checkedAll = true
	return s.err
}
func (s *quotaServiceStub) CheckCredential(_ context.Context, credentialID string) error {
	s.lastCredential = credentialID
	return s.err
}
func (s *quotaServiceStub) GetCredentialState(_ context.Context, _ string) (*domainquota.CredentialState, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.state, nil
}
func (s *quotaServiceStub) NeedsRehydration(_ context.Context) (bool, error) { return false, s.err }

func TestRefreshHandler_Refresh_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewRefreshHandler(&refreshServiceStub{}, nil)
	r := gin.New()
	r.POST("/v1/internal/auth-profiles/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/auth-profiles/refresh", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRefreshHandler_Refresh_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewRefreshHandler(&refreshServiceStub{err: errors.New("refresh failed")}, nil)
	r := gin.New()
	r.POST("/v1/internal/auth-profiles/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/auth-profiles/refresh", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRefreshHandler_Refresh_CredentialID_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	quotaSvc := &quotaServiceStub{state: &domainquota.CredentialState{
		Status:          domainquota.StatusHealthy,
		Quota:           domainquota.NewQuotaInfo(238, 300),
		QuotaDetail:     &domainquota.CopilotQuotaSnapshots{PremiumInteractions: &domainquota.CopilotQuotaSnapshot{QuotaID: "premium_interactions"}},
		AccessTokenHash: "hash-1",
	}}
	refreshSvc := &refreshServiceStub{}
	h := NewRefreshHandler(refreshSvc, quotaSvc)
	r := gin.New()
	r.POST("/v1/internal/auth-profiles/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/auth-profiles/refresh?credential_id=cred-1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if quotaSvc.lastCredential != "cred-1" {
		t.Fatalf("quota check credential_id = %q, want cred-1", quotaSvc.lastCredential)
	}
	if refreshSvc.lastCredentialID != "cred-1" {
		t.Fatalf("refresh credential_id = %q, want cred-1", refreshSvc.lastCredentialID)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["credential_id"] != "cred-1" {
		t.Fatalf("response credential_id = %v, want cred-1", body["credential_id"])
	}

	if body["status"] != string(domainquota.StatusHealthy) {
		t.Fatalf("status = %v, want %s", body["status"], domainquota.StatusHealthy)
	}
	if _, ok := body["access_token_hash"]; !ok {
		t.Fatal("expected access_token_hash in response")
	}
	if _, ok := body["quota"]; !ok {
		t.Fatal("expected quota in response")
	}
}

func TestRefreshHandler_Refresh_TriggersQuotaCheckAll(t *testing.T) {
	gin.SetMode(gin.TestMode)

	quotaSvc := &quotaServiceStub{}
	h := NewRefreshHandler(&refreshServiceStub{}, quotaSvc)
	r := gin.New()
	r.POST("/v1/internal/auth-profiles/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/auth-profiles/refresh", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !quotaSvc.checkedAll {
		t.Fatal("expected quota CheckAll to be called")
	}
}

func TestRefreshHandler_Refresh_CredentialID_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)

	refreshSvc := &refreshServiceStub{err: errors.New("failed refresh")}
	quotaSvc := &quotaServiceStub{}
	h := NewRefreshHandler(refreshSvc, quotaSvc)
	r := gin.New()
	r.POST("/v1/internal/auth-profiles/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/auth-profiles/refresh?credential_id=cred-1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if quotaSvc.lastCredential != "" {
		t.Fatalf("quota check should not run on refresh error, got %q", quotaSvc.lastCredential)
	}
}

func TestRefreshHandler_Refresh_CredentialID_QuotaFailureStillReturnsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	refreshSvc := &refreshServiceStub{}
	quotaSvc := &quotaServiceStub{err: errors.New("quota failure")}
	h := NewRefreshHandler(refreshSvc, quotaSvc)
	r := gin.New()
	r.POST("/v1/internal/auth-profiles/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/auth-profiles/refresh?credential_id=cred-1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if refreshSvc.lastCredentialID != "cred-1" {
		t.Fatalf("refresh credential_id = %q, want cred-1", refreshSvc.lastCredentialID)
	}
	if quotaSvc.lastCredential != "cred-1" {
		t.Fatalf("quota credential_id = %q, want cred-1", quotaSvc.lastCredential)
	}
}
