package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	"github.com/gin-gonic/gin"
)

type copilotDeviceFlowCoordinatorStub struct {
	startResp    domainoauth.DeviceFlowResponse
	startErr     error
	status       *domainoauth.OAuthSession
	statusErr    error
	lastDeviceID string
}

func (s *copilotDeviceFlowCoordinatorStub) StartDeviceFlow(context.Context) (domainoauth.DeviceFlowResponse, error) {
	if s.startErr != nil {
		return domainoauth.DeviceFlowResponse{}, s.startErr
	}
	return s.startResp, nil
}

func (s *copilotDeviceFlowCoordinatorStub) GetDeviceStatus(_ context.Context, deviceCode string) (*domainoauth.OAuthSession, error) {
	s.lastDeviceID = deviceCode
	if s.statusErr != nil {
		return nil, s.statusErr
	}
	return s.status, nil
}

func TestCopilotOAuthHandler_StartDeviceFlow_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	coordinator := &copilotDeviceFlowCoordinatorStub{
		startResp: domainoauth.DeviceFlowResponse{
			DeviceCode:      "github-device-code-123",
			UserCode:        "WXYZ-1234",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        5,
		},
	}

	handler := NewCopilotOAuthHandler(coordinator)
	router := gin.New()
	router.POST("/v1/internal/oauth/copilot-device-code", handler.StartDeviceFlow)

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/oauth/copilot-device-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
	if body["device_code"] != "github-device-code-123" {
		t.Errorf("expected device code github-device-code-123, got %v", body["device_code"])
	}
	if body["user_code"] != "WXYZ-1234" {
		t.Errorf("expected user code WXYZ-1234, got %v", body["user_code"])
	}
}

func TestCopilotOAuthHandler_StartDeviceFlow_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewCopilotOAuthHandler(&copilotDeviceFlowCoordinatorStub{
		startErr: errors.New("device flow disabled"),
	})
	router := gin.New()
	router.POST("/v1/internal/oauth/copilot-device-code", handler.StartDeviceFlow)

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/oauth/copilot-device-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_MissingDeviceCode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewCopilotOAuthHandler(&copilotDeviceFlowCoordinatorStub{})
	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_PendingWhenNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	coordinator := &copilotDeviceFlowCoordinatorStub{}
	handler := NewCopilotOAuthHandler(coordinator)
	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["status"] != "wait" {
		t.Errorf("expected status wait, got %v", body["status"])
	}
	if coordinator.lastDeviceID != "test-code" {
		t.Errorf("expected device code test-code, got %s", coordinator.lastDeviceID)
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_PendingSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewCopilotOAuthHandler(&copilotDeviceFlowCoordinatorStub{
		status: &domainoauth.OAuthSession{State: domainoauth.StatePending},
	})
	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["status"] != "wait" {
		t.Errorf("expected status wait, got %v", body["status"])
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	expiresAt := time.Now().Add(30 * time.Minute)
	lastRefreshAt := time.Now()
	handler := NewCopilotOAuthHandler(&copilotDeviceFlowCoordinatorStub{
		status: &domainoauth.OAuthSession{
			State:     domainoauth.StateOK,
			AccountID: "testuser",
			Connection: &domainoauth.ConnectionSummary{
				ID:            "conn-1",
				AccountID:     "testuser",
				Email:         "test@example.com",
				Provider:      "copilot",
				ExpiresAt:     &expiresAt,
				LastRefreshAt: &lastRefreshAt,
				Enabled:       true,
			},
		},
	})
	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-device-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
	if body["account_id"] != "testuser" {
		t.Errorf("expected account_id testuser, got %v", body["account_id"])
	}
	connection, ok := body["connection"].(map[string]any)
	if !ok {
		t.Fatalf("expected connection payload, got %T", body["connection"])
	}
	if connection["provider"] != "copilot" {
		t.Errorf("expected provider copilot, got %v", connection["provider"])
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_ErrorState(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewCopilotOAuthHandler(&copilotDeviceFlowCoordinatorStub{
		status: &domainoauth.OAuthSession{
			State:        domainoauth.StateError,
			ErrorCode:    "no_subscription",
			ErrorMessage: "GitHub Copilot subscription required",
		},
	})
	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-device-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["status"] != "error" {
		t.Errorf("expected status error, got %v", body["status"])
	}
	if body["error_code"] != "no_subscription" {
		t.Errorf("expected error_code no_subscription, got %v", body["error_code"])
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_LookupError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewCopilotOAuthHandler(&copilotDeviceFlowCoordinatorStub{
		statusErr: errors.New("redis unavailable"),
	})
	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-device-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}
}
