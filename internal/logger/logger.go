package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// Level represents log level (kept for API compatibility)
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var (
	defaultLogger *log.Logger
	logFile       *os.File
	enabled       bool
	mu            sync.Mutex
	once          sync.Once
)

// Init initializes the default logger
func Init(minLevel Level) error {
	var initErr error
	once.Do(func() {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			initErr = fmt.Errorf("get home dir: %w", err)
			return
		}

		logDir := filepath.Join(homeDir, ".k4s", "logs")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			initErr = fmt.Errorf("create log dir: %w", err)
			return
		}

		// Create log file with date
		logPath := filepath.Join(logDir, fmt.Sprintf("k4s-%s.log", time.Now().Format("2006-01-02")))
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			initErr = fmt.Errorf("open log file: %w", err)
			return
		}
		logFile = f

		defaultLogger = log.NewWithOptions(f, log.Options{
			Level:           toCharmLevel(minLevel),
			ReportTimestamp: true,
			TimeFormat:      "2006-01-02 15:04:05.000",
		})
		// File output — no colors
		defaultLogger.SetColorProfile(0)

		enabled = true

		// Write startup message
		defaultLogger.Info("k4s logger initialized")
	})
	return initErr
}

// Close closes the logger
func Close() {
	if logFile != nil {
		logFile.Close()
	}
}

// SetEnabled enables or disables logging
func SetEnabled(e bool) {
	mu.Lock()
	defer mu.Unlock()
	enabled = e
}

// Debug logs a debug message with structured key-value pairs
func Debug(msg string, keyvals ...interface{}) {
	if defaultLogger != nil && enabled {
		defaultLogger.Debug(msg, keyvals...)
	}
}

// Info logs an info message with structured key-value pairs
func Info(msg string, keyvals ...interface{}) {
	if defaultLogger != nil && enabled {
		defaultLogger.Info(msg, keyvals...)
	}
}

// Warn logs a warning message with structured key-value pairs
func Warn(msg string, keyvals ...interface{}) {
	if defaultLogger != nil && enabled {
		defaultLogger.Warn(msg, keyvals...)
	}
}

// Error logs an error message with structured key-value pairs
func Error(msg string, keyvals ...interface{}) {
	if defaultLogger != nil && enabled {
		defaultLogger.Error(msg, keyvals...)
	}
}

func toCharmLevel(l Level) log.Level {
	switch l {
	case LevelDebug:
		return log.DebugLevel
	case LevelInfo:
		return log.InfoLevel
	case LevelWarn:
		return log.WarnLevel
	case LevelError:
		return log.ErrorLevel
	default:
		return log.DebugLevel
	}
}
