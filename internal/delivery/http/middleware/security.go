package middleware

import (
	"bytes"
	"io"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SensitiveParams are OAuth/security-related query parameters and form fields to redact
var SensitiveParams = []string{
	"code",
	"state",
	"token",
	"access_token",
	"refresh_token",
	"id_token",
	"code_verifier",
	"code_challenge",
	"device_code",
	"user_code",
}

// RedactedValue is the placeholder for redacted sensitive data
const RedactedValue = "[REDACTED]"

// SecurityLogger creates middleware that logs requests with sensitive data redaction
func SecurityLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Redact query params
		redactedPath := redactQueryParams(c.Request.URL.Path, c.Request.URL.RawQuery)

		c.Next()

		logger.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", redactedPath),
			zap.Int("status", c.Writer.Status()),
		)
	}
}

// redactQueryParams replaces sensitive query parameter values with [REDACTED]
func redactQueryParams(path string, rawQuery string) string {
	if rawQuery == "" {
		return path
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return path + "?" + rawQuery
	}

	for _, param := range SensitiveParams {
		if values.Has(param) {
			values.Set(param, RedactedValue)
		}
	}

	return path + "?" + values.Encode()
}

// RedactFromBody searches for sensitive data patterns in request body and redacts them
func RedactFromBody(body []byte) string {
	content := string(body)

	// Redact JSON patterns: "param":"value"
	for _, param := range SensitiveParams {
		quotePattern := `"` + param + `":`
		searchFrom := 0
		for {
			idx := strings.Index(content[searchFrom:], quotePattern)
			if idx == -1 {
				break
			}
			idx += searchFrom // Adjust to absolute position
			valueStart := idx + len(quotePattern)
			// Skip whitespace
			for valueStart < len(content) && (content[valueStart] == ' ' || content[valueStart] == '\t') {
				valueStart++
			}
			// Check if value is quoted
			if valueStart < len(content) && content[valueStart] == '"' {
				// Find closing quote
				valueEnd := valueStart + 1
				for valueEnd < len(content) && content[valueEnd] != '"' {
					if content[valueEnd] == '\\' {
						valueEnd++ // Skip escaped character
						if valueEnd < len(content) {
							valueEnd++
						}
					} else {
						valueEnd++
					}
				}
				if valueEnd < len(content) {
					// Replace value between quotes
					content = content[:valueStart+1] + RedactedValue + content[valueEnd:]
					// Move search position past the redaction
					searchFrom = valueStart + 1 + len(RedactedValue)
				} else {
					break
				}
			} else {
				// No quote found, move past this match
				searchFrom = valueStart
			}
		}

		// Pattern: param=value (form-encoded)
		formPattern := param + `=`
		searchFrom = 0
		for {
			idx := strings.Index(content[searchFrom:], formPattern)
			if idx == -1 {
				break
			}
			idx += searchFrom
			valueStart := idx + len(formPattern)
			// Find end of value
			valueEnd := valueStart
			for valueEnd < len(content) && content[valueEnd] != '&' && content[valueEnd] != ' ' && content[valueEnd] != '\n' && content[valueEnd] != '\r' {
				valueEnd++
			}
			// Replace value
			content = content[:valueStart] + RedactedValue + content[valueEnd:]
			// Move search position past the redaction
			searchFrom = valueStart + len(RedactedValue)
		}
	}

	return content
}

// LogSafeBody returns a redacted version of request body for logging
// Use this instead of directly logging c.Request.Body
func LogSafeBody(c *gin.Context) string {
	if c.Request.Body == nil {
		return ""
	}

	// Read body
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return "[error reading body]"
	}

	// Restore body for downstream handlers
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Redact and return
	return RedactFromBody(bodyBytes)
}
