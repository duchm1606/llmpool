package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSConfig holds configuration for CORS middleware.
type CORSConfig struct {
	AllowedOrigins []string // List of allowed origins, e.g., ["http://localhost:3000", "https://dashboard.example.com"]
	AllowedMethods []string // List of allowed methods, e.g., ["GET", "POST", "PUT", "DELETE"]
	AllowedHeaders []string // List of allowed headers
	MaxAge         int      // Preflight cache duration in seconds
}

// DefaultCORSConfig returns sensible defaults.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID"},
		MaxAge:         86400, // 24 hours
	}
}

// CORS returns a CORS middleware with the given configuration.
func CORS(config CORSConfig) gin.HandlerFunc {
	// Build lookup set for allowed origins
	allowedOriginsSet := make(map[string]bool)
	allowAll := false
	for _, origin := range config.AllowedOrigins {
		if origin == "*" {
			allowAll = true
		}
		allowedOriginsSet[origin] = true
	}

	allowedMethods := strings.Join(config.AllowedMethods, ", ")
	allowedHeaders := strings.Join(config.AllowedHeaders, ", ")

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// Check if origin is allowed
		if origin != "" {
			if allowAll || allowedOriginsSet[origin] {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Methods", allowedMethods)
				c.Header("Access-Control-Allow-Headers", allowedHeaders)
				c.Header("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
				c.Header("Access-Control-Allow-Credentials", "true")
			}
		}

		// Handle preflight requests
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// CORSForRoutes applies CORS middleware only to specific route prefixes.
func CORSForRoutes(config CORSConfig, prefixes ...string) gin.HandlerFunc {
	corsHandler := CORS(config)

	return func(c *gin.Context) {
		for _, prefix := range prefixes {
			if strings.HasPrefix(c.Request.URL.Path, prefix) {
				corsHandler(c)
				return
			}
		}
		c.Next()
	}
}
