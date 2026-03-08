package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSForRoutes_OnlyConfiguredPrefixes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(CORSForRoutes(
		CORSConfig{
			AllowedOrigins: []string{"http://localhost:3000"},
			AllowedMethods: []string{"GET", "POST", "PATCH", "OPTIONS"},
			AllowedHeaders: []string{"Content-Type"},
			MaxAge:         600,
		},
		"/v1/internal/usage",
		"/v1/internal/oauth",
		"/v1/internal/auth-profiles",
	))
	r.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/v1/internal/usage", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/v1/internal/auth-profiles", func(c *gin.Context) { c.Status(http.StatusOK) })

	reqInternal := httptest.NewRequest(http.MethodGet, "/v1/internal/auth-profiles", nil)
	reqInternal.Header.Set("Origin", "http://localhost:3000")
	wInternal := httptest.NewRecorder()
	r.ServeHTTP(wInternal, reqInternal)

	if got := wInternal.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("internal route CORS origin = %q, want http://localhost:3000", got)
	}

	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	reqHealth.Header.Set("Origin", "http://localhost:3000")
	wHealth := httptest.NewRecorder()
	r.ServeHTTP(wHealth, reqHealth)

	if got := wHealth.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("non-internal route should not have CORS header, got %q", got)
	}
}
