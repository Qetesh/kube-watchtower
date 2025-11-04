package logger

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/term"
)

var (
	log          *zap.SugaredLogger
	colorEnabled bool
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorPurple = "\033[35m"

	// Bright colors
	colorBrightRed = "\033[91m"
)

// coloredWriteSyncer wraps zapcore.WriteSyncer to add colors to entire log lines
type coloredWriteSyncer struct {
	zapcore.WriteSyncer
	colorEnabled bool
}

// Write intercepts the output and adds color to the entire line based on log level
func (c *coloredWriteSyncer) Write(p []byte) (n int, err error) {
	if !c.colorEnabled {
		return c.WriteSyncer.Write(p)
	}

	line := string(p)
	var color string

	// Detect log level in the line and choose color
	if strings.Contains(line, "DEBUG") {
		color = colorPurple
	} else if strings.Contains(line, "INFO") {
		color = colorGreen
	} else if strings.Contains(line, "WARN") {
		color = colorYellow
	} else if strings.Contains(line, "ERROR") {
		color = colorRed
	} else if strings.Contains(line, "FATAL") || strings.Contains(line, "PANIC") {
		color = colorBrightRed
	}

	// Add color to entire line
	if color != "" {
		// Remove trailing newline, add color, then add newline back
		line = strings.TrimSuffix(line, "\n")
		line = color + line + colorReset + "\n"
	}

	return c.WriteSyncer.Write([]byte(line))
}

// Init initializes the logger with the specified level
func Init(level string) error {
	zapLevel := parseLevel(level)

	// Determine color usage based on LOG_COLOR environment variable
	// Values: "always" (default), "never", "auto"
	colorMode := strings.ToLower(os.Getenv("LOG_COLOR"))
	switch colorMode {
	case "never", "false", "0", "no":
		colorEnabled = false
	case "auto":
		// Auto-detect terminal
		colorEnabled = term.IsTerminal(int(os.Stderr.Fd()))
	default:
		// Default to always enabled (for container environments)
		colorEnabled = true
	}

	// Configure encoder
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		MessageKey:     "msg",
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05"),
		EncodeDuration: zapcore.StringDurationEncoder,
		LineEnding:     zapcore.DefaultLineEnding,
	}

	// Create write syncer with color support
	var ws zapcore.WriteSyncer
	if colorEnabled {
		ws = &coloredWriteSyncer{
			WriteSyncer:  zapcore.AddSync(os.Stderr),
			colorEnabled: colorEnabled,
		}
	} else {
		ws = zapcore.AddSync(os.Stderr)
	}

	// Create core
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		ws,
		zapLevel,
	)

	logger := zap.New(core)
	log = logger.Sugar()

	return nil
}

// parseLevel parses log level string
func parseLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// Get returns the global logger
func Get() *zap.SugaredLogger {
	if log == nil {
		// Fallback to default logger if not initialized
		_ = Init("info")
	}
	return log
}

// Debug logs a debug message
func Debug(args ...interface{}) {
	Get().Debug(args...)
}

// Debugf logs a formatted debug message
func Debugf(template string, args ...interface{}) {
	Get().Debugf(template, args...)
}

// Info logs an info message
func Info(args ...interface{}) {
	Get().Info(args...)
}

// Infof logs a formatted info message
func Infof(template string, args ...interface{}) {
	Get().Infof(template, args...)
}

// Warn logs a warning message
func Warn(args ...interface{}) {
	Get().Warn(args...)
}

// Warnf logs a formatted warning message
func Warnf(template string, args ...interface{}) {
	Get().Warnf(template, args...)
}

// Error logs an error message
func Error(args ...interface{}) {
	Get().Error(args...)
}

// Errorf logs a formatted error message
func Errorf(template string, args ...interface{}) {
	Get().Errorf(template, args...)
}

// Fatal logs a fatal message and exits
func Fatal(args ...interface{}) {
	Get().Fatal(args...)
}

// Fatalf logs a formatted fatal message and exits
func Fatalf(template string, args ...interface{}) {
	Get().Fatalf(template, args...)
}

// Sync flushes any buffered log entries
func Sync() error {
	if log != nil {
		return log.Sync()
	}
	return nil
}
