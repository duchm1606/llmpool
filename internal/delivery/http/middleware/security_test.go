package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestSecurityLogger_RedactsQueryParams(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		query            string
		wantLogPath      string
		shouldNotContain []string
	}{
		{
			name:             "redacts OAuth code",
			path:             "/v1/internal/oauth/callback",
			query:            "code=secret123&state=xyz789",
			wantLogPath:      "/v1/internal/oauth/callback?code=%5BREDACTED%5D&state=%5BREDACTED%5D",
			shouldNotContain: []string{"secret123", "xyz789"},
		},
		{
			name:             "redacts access_token",
			path:             "/v1/internal/oauth/status",
			query:            "access_token=tok_abc123&foo=bar",
			wantLogPath:      "/v1/internal/oauth/status?access_token=%5BREDACTED%5D&foo=bar",
			shouldNotContain: []string{"tok_abc123"},
		},
		{
			name:             "redacts refresh_token",
			path:             "/v1/internal/auth-profiles/refresh",
			query:            "refresh_token=refresh_secret",
			wantLogPath:      "/v1/internal/auth-profiles/refresh?refresh_token=%5BREDACTED%5D",
			shouldNotContain: []string{"refresh_secret"},
		},
		{
			name:             "redacts device_code",
			path:             "/v1/internal/oauth/device/poll",
			query:            "device_code=device_xyz&user_code=ABCD-1234",
			wantLogPath:      "/v1/internal/oauth/device/poll?device_code=%5BREDACTED%5D&user_code=%5BREDACTED%5D",
			shouldNotContain: []string{"device_xyz", "ABCD-1234"},
		},
		{
			name:             "redacts code_verifier and code_challenge",
			path:             "/oauth/verify",
			query:            "code_verifier=verifier123&code_challenge=challenge456",
			wantLogPath:      "/oauth/verify?code_challenge=%5BREDACTED%5D&code_verifier=%5BREDACTED%5D",
			shouldNotContain: []string{"verifier123", "challenge456"},
		},
		{
			name:             "preserves non-sensitive params",
			path:             "/v1/internal/oauth/status",
			query:            "session_id=sess_123&foo=bar",
			wantLogPath:      "/v1/internal/oauth/status?foo=bar&session_id=sess_123",
			shouldNotContain: []string{},
		},
		{
			name:             "handles empty query",
			path:             "/health",
			query:            "",
			wantLogPath:      "/health",
			shouldNotContain: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create observed logger
			core, logs := observer.New(zapcore.InfoLevel)
			logger := zap.New(core)

			// Setup Gin router
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(SecurityLogger(logger))
			r.GET(tt.path, func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			// Create request
			url := tt.path
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			// Execute request
			r.ServeHTTP(w, req)

			// Verify logs
			if logs.Len() == 0 {
				t.Fatal("expected log entry, got none")
			}

			entry := logs.All()[0]
			var loggedPath string
			for _, field := range entry.Context {
				if field.Key == "path" {
					loggedPath = field.String
					break
				}
			}

			// Check redacted path matches expected
			if loggedPath != tt.wantLogPath {
				t.Errorf("logged path = %q, want %q", loggedPath, tt.wantLogPath)
			}

			// Ensure sensitive values are NOT in logs
			allLogs := logs.All()
			for _, log := range allLogs {
				logStr := log.Message
				for _, field := range log.Context {
					logStr += " " + field.String
				}
				for _, sensitive := range tt.shouldNotContain {
					if strings.Contains(logStr, sensitive) {
						t.Errorf("log contains sensitive data %q: %s", sensitive, logStr)
					}
				}
			}
		})
	}
}

func TestRedactFromBody_JSONPayload(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		shouldNotContain []string
		shouldContain    []string
	}{
		{
			name:             "redacts access_token in JSON",
			body:             `{"access_token":"secret_token_abc","user":"john"}`,
			shouldNotContain: []string{"secret_token_abc"},
			shouldContain:    []string{"access_token", "[REDACTED]", "john"},
		},
		{
			name:             "redacts refresh_token in JSON",
			body:             `{"refresh_token":"refresh_xyz","expires_in":3600}`,
			shouldNotContain: []string{"refresh_xyz"},
			shouldContain:    []string{"refresh_token", "[REDACTED]", "3600"},
		},
		{
			name:             "redacts code in JSON",
			body:             `{"code":"auth_code_123","state":"state_456"}`,
			shouldNotContain: []string{"auth_code_123", "state_456"},
			shouldContain:    []string{"code", "state", "[REDACTED]"},
		},
		{
			name:             "preserves non-sensitive data",
			body:             `{"username":"alice","email":"alice@example.com"}`,
			shouldNotContain: []string{},
			shouldContain:    []string{"username", "alice", "email", "alice@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redacted := RedactFromBody([]byte(tt.body))

			// Verify sensitive data is redacted
			for _, sensitive := range tt.shouldNotContain {
				if strings.Contains(redacted, sensitive) {
					t.Errorf("redacted body contains sensitive data %q: %s", sensitive, redacted)
				}
			}

			// Verify expected content is present
			for _, expected := range tt.shouldContain {
				if !strings.Contains(redacted, expected) {
					t.Errorf("redacted body missing expected content %q: %s", expected, redacted)
				}
			}
		})
	}
}

func TestRedactFromBody_FormData(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		shouldNotContain []string
		shouldContain    []string
	}{
		{
			name:             "redacts code in form data",
			body:             "code=auth_code_123&state=xyz&foo=bar",
			shouldNotContain: []string{"auth_code_123", "xyz"},
			shouldContain:    []string{"code", "state", "[REDACTED]", "foo", "bar"},
		},
		{
			name:             "redacts token in form data",
			body:             "token=secret_tok&username=alice",
			shouldNotContain: []string{"secret_tok"},
			shouldContain:    []string{"token", "[REDACTED]", "username", "alice"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redacted := RedactFromBody([]byte(tt.body))

			for _, sensitive := range tt.shouldNotContain {
				if strings.Contains(redacted, sensitive) {
					t.Errorf("redacted body contains sensitive data %q: %s", sensitive, redacted)
				}
			}

			for _, expected := range tt.shouldContain {
				if !strings.Contains(redacted, expected) {
					t.Errorf("redacted body missing expected content %q: %s", expected, redacted)
				}
			}
		})
	}
}

func TestLogSafeBody(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		shouldNotContain []string
	}{
		{
			name:             "redacts OAuth credentials",
			body:             `{"access_token":"tok_secret","refresh_token":"refresh_secret"}`,
			shouldNotContain: []string{"tok_secret", "refresh_secret"},
		},
		{
			name:             "handles empty body",
			body:             "",
			shouldNotContain: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup Gin context
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Set request body
			c.Request = httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(tt.body))

			// Get safe body
			safeBody := LogSafeBody(c)

			// Verify sensitive data is redacted
			for _, sensitive := range tt.shouldNotContain {
				if strings.Contains(safeBody, sensitive) {
					t.Errorf("safe body contains sensitive data %q: %s", sensitive, safeBody)
				}
			}

			// Verify body can still be read by downstream handlers
			bodyBytes, err := io.ReadAll(c.Request.Body)
			if err != nil {
				t.Fatalf("failed to read restored body: %v", err)
			}
			if string(bodyBytes) != tt.body {
				t.Errorf("body not properly restored: got %q, want %q", string(bodyBytes), tt.body)
			}
		})
	}
}

func TestSecurityLogger_IntegrationWithOAuthCallback(t *testing.T) {
	// This test simulates a full OAuth callback request with sensitive data
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityLogger(logger))
	r.GET("/v1/internal/oauth/callback", func(c *gin.Context) {
		// Simulate OAuth callback handler
		code := c.Query("code")
		state := c.Query("state")

		// Handler should still have access to unredacted values
		if code == "" || state == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing parameters"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "processing"})
	})

	// Create request with sensitive OAuth parameters
	req := httptest.NewRequest(http.MethodGet,
		"/v1/internal/oauth/callback?code=oauth_secret_code_12345&state=random_state_67890",
		nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Verify handler received the request successfully
	if w.Code != http.StatusOK {
		t.Errorf("handler failed: status = %d", w.Code)
	}

	// Verify logs are redacted
	if logs.Len() == 0 {
		t.Fatal("expected log entry")
	}

	allLogs := logs.All()
	for _, entry := range allLogs {
		logContent := entry.Message
		for _, field := range entry.Context {
			logContent += " " + field.String
		}

		// Ensure sensitive values are NOT in logs
		if strings.Contains(logContent, "oauth_secret_code_12345") {
			t.Errorf("log contains OAuth code: %s", logContent)
		}
		if strings.Contains(logContent, "random_state_67890") {
			t.Errorf("log contains state: %s", logContent)
		}

		// Ensure [REDACTED] or %5BREDACTED%5D (URL-encoded) is present
		if !strings.Contains(logContent, "[REDACTED]") && !strings.Contains(logContent, "%5BREDACTED%5D") {
			t.Errorf("log does not contain [REDACTED] marker: %s", logContent)
		}
	}
}
