package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type refreshServiceStub struct {
	err error
}

func (s refreshServiceStub) RefreshDue(_ context.Context) error {
	return s.err
}

func TestRefreshHandler_Refresh_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewRefreshHandler(refreshServiceStub{})
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

	h := NewRefreshHandler(refreshServiceStub{err: errors.New("refresh failed")})
	r := gin.New()
	r.POST("/v1/internal/auth-profiles/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/auth-profiles/refresh", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
