package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/oronno/privateledger/internal/config"
)

// Setup configures the global slog logger based on the provided configuration
func Setup(cfg *config.Config, execDir string) (*os.File, error) {
	// Parse log level
	level := parseLogLevel(cfg.Logging.LogLevel)

	var writer io.Writer = os.Stdout
	var logFile *os.File

	// If file logging is enabled, create a multi-writer
	if cfg.Logging.EnableFileLogging {
		logPath := cfg.Logging.LogFilePath
		if !filepath.IsAbs(logPath) {
			logPath = filepath.Join(execDir, logPath)
		}

		var err error
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}

		// Write to both stdout and file
		writer = io.MultiWriter(os.Stdout, logFile)
	}

	// Create handler with JSON format for better structured logging
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: level,
	})

	// Set as default logger
	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logFile, nil
}

// parseLogLevel converts string log level to slog.Level
func parseLogLevel(levelStr string) slog.Level {
	switch strings.ToLower(levelStr) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
