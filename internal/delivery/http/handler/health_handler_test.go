package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	usecasehealth "github.com/duchoang/llmpool/internal/usecase/health"
	"github.com/gin-gonic/gin"
)

func TestHealthHandler_Get(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	h := NewHealthHandler(usecasehealth.NewService())
	r.GET("/health", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status code 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "ok" {
		t.Fatalf("expected body status=ok, got %q", body["status"])
	}
}
