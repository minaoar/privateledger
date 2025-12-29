package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// responseWriter wraps gin.ResponseWriter to capture the response body
type responseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// LoggingMiddleware logs all HTTP requests and responses
func LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Read request body
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			// Restore the body for handlers to read
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Wrap response writer to capture response body
		respWriter := &responseWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBufferString(""),
		}
		c.Writer = respWriter

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)

		// Prepare log attributes
		attrs := []any{
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.String("query", c.Request.URL.RawQuery),
			slog.Int("status", c.Writer.Status()),
			slog.String("ip", c.ClientIP()),
			slog.String("user_agent", c.Request.UserAgent()),
			slog.Duration("duration", duration),
		}

		// Add request body if present (limit size to avoid huge logs)
		if len(requestBody) > 0 && len(requestBody) < 500 {
			attrs = append(attrs, slog.String("request_body", string(requestBody)))
		} else if len(requestBody) > 0 {
			attrs = append(attrs, slog.Int("request_body_size", len(requestBody)))
		}

		// Add response body if present and not too large
		responseBody := respWriter.body.String()
		if len(responseBody) > 0 && len(responseBody) < 500 {
			attrs = append(attrs, slog.String("response_body", responseBody))
		} else if len(responseBody) > 0 {
			attrs = append(attrs, slog.Int("response_body_size", len(responseBody)))
		}

		// Add errors if any
		if len(c.Errors) > 0 {
			attrs = append(attrs, slog.String("errors", c.Errors.String()))
		}

		// Log based on status code
		if c.Writer.Status() >= 500 {
			slog.Error("HTTP Request", attrs...)
		} else if c.Writer.Status() >= 400 {
			slog.Warn("HTTP Request", attrs...)
		} else {
			slog.Info("HTTP Request", attrs...)
		}
	}
}
