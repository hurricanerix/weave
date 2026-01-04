package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	output := &bytes.Buffer{}
	logger := New(LevelInfo, output)

	if logger == nil {
		t.Fatal("New() returned nil")
	}
	if logger.level != LevelInfo {
		t.Errorf("level = %v, want %v", logger.level, LevelInfo)
	}
}

func TestNewFromString(t *testing.T) {
	tests := []struct {
		name      string
		levelStr  string
		wantLevel Level
	}{
		{"debug", "debug", LevelDebug},
		{"info", "info", LevelInfo},
		{"warn", "warn", LevelWarn},
		{"error", "error", LevelError},
		{"DEBUG uppercase", "DEBUG", LevelDebug},
		{"INFO uppercase", "INFO", LevelInfo},
		{"unknown defaults to info", "invalid", LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			logger := NewFromString(tt.levelStr, output)

			if logger.level != tt.wantLevel {
				t.Errorf("level = %v, want %v", logger.level, tt.wantLevel)
			}
		})
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Level
	}{
		{"debug", "debug", LevelDebug},
		{"info", "info", LevelInfo},
		{"warn", "warn", LevelWarn},
		{"error", "error", LevelError},
		{"DEBUG uppercase", "DEBUG", LevelDebug},
		{"unknown", "unknown", LevelInfo},
		{"empty", "", LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLevel(tt.input)
			if got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.level.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	tests := []struct {
		name       string
		logLevel   Level
		logFunc    func(*Logger)
		wantOutput bool
	}{
		// Debug level - logs everything
		{"debug level logs debug", LevelDebug, func(l *Logger) { l.Debug("test") }, true},
		{"debug level logs info", LevelDebug, func(l *Logger) { l.Info("test") }, true},
		{"debug level logs warn", LevelDebug, func(l *Logger) { l.Warn("test") }, true},
		{"debug level logs error", LevelDebug, func(l *Logger) { l.Error("test") }, true},

		// Info level - filters debug
		{"info level filters debug", LevelInfo, func(l *Logger) { l.Debug("test") }, false},
		{"info level logs info", LevelInfo, func(l *Logger) { l.Info("test") }, true},
		{"info level logs warn", LevelInfo, func(l *Logger) { l.Warn("test") }, true},
		{"info level logs error", LevelInfo, func(l *Logger) { l.Error("test") }, true},

		// Warn level - filters debug and info
		{"warn level filters debug", LevelWarn, func(l *Logger) { l.Debug("test") }, false},
		{"warn level filters info", LevelWarn, func(l *Logger) { l.Info("test") }, false},
		{"warn level logs warn", LevelWarn, func(l *Logger) { l.Warn("test") }, true},
		{"warn level logs error", LevelWarn, func(l *Logger) { l.Error("test") }, true},

		// Error level - only logs errors
		{"error level filters debug", LevelError, func(l *Logger) { l.Debug("test") }, false},
		{"error level filters info", LevelError, func(l *Logger) { l.Info("test") }, false},
		{"error level filters warn", LevelError, func(l *Logger) { l.Warn("test") }, false},
		{"error level logs error", LevelError, func(l *Logger) { l.Error("test") }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			logger := New(tt.logLevel, output)

			tt.logFunc(logger)

			gotOutput := output.Len() > 0
			if gotOutput != tt.wantOutput {
				t.Errorf("output present = %v, want %v", gotOutput, tt.wantOutput)
			}
		})
	}
}

func TestLogger_Debug(t *testing.T) {
	output := &bytes.Buffer{}
	logger := New(LevelDebug, output)

	logger.Debug("test message: %s", "hello")

	got := output.String()
	if !strings.Contains(got, "[DEBUG]") {
		t.Errorf("output missing [DEBUG]: %q", got)
	}
	if !strings.Contains(got, "test message: hello") {
		t.Errorf("output missing message: %q", got)
	}
}

func TestLogger_Info(t *testing.T) {
	output := &bytes.Buffer{}
	logger := New(LevelInfo, output)

	logger.Info("test message: %s", "hello")

	got := output.String()
	if !strings.Contains(got, "[INFO]") {
		t.Errorf("output missing [INFO]: %q", got)
	}
	if !strings.Contains(got, "test message: hello") {
		t.Errorf("output missing message: %q", got)
	}
}

func TestLogger_Warn(t *testing.T) {
	output := &bytes.Buffer{}
	logger := New(LevelWarn, output)

	logger.Warn("test message: %s", "hello")

	got := output.String()
	if !strings.Contains(got, "[WARN]") {
		t.Errorf("output missing [WARN]: %q", got)
	}
	if !strings.Contains(got, "test message: hello") {
		t.Errorf("output missing message: %q", got)
	}
}

func TestLogger_Error(t *testing.T) {
	output := &bytes.Buffer{}
	logger := New(LevelError, output)

	logger.Error("test message: %s", "hello")

	got := output.String()
	if !strings.Contains(got, "[ERROR]") {
		t.Errorf("output missing [ERROR]: %q", got)
	}
	if !strings.Contains(got, "test message: hello") {
		t.Errorf("output missing message: %q", got)
	}
}

func TestLogger_SetLevel(t *testing.T) {
	output := &bytes.Buffer{}
	logger := New(LevelInfo, output)

	// Initially at Info level
	logger.Debug("should not appear")
	if output.Len() > 0 {
		t.Error("debug message logged at info level")
	}

	// Change to Debug level
	logger.SetLevel(LevelDebug)
	logger.Debug("should appear")
	if output.Len() == 0 {
		t.Error("debug message not logged at debug level")
	}
}

func TestLogger_GetLevel(t *testing.T) {
	logger := New(LevelWarn, nil)

	got := logger.GetLevel()
	if got != LevelWarn {
		t.Errorf("GetLevel() = %v, want %v", got, LevelWarn)
	}
}

func TestNew_NilOutput(t *testing.T) {
	// Should not panic with nil output (uses os.Stderr)
	logger := New(LevelInfo, nil)
	if logger == nil {
		t.Fatal("New() returned nil with nil output")
	}

	// Should not panic when logging
	logger.Info("test")
}
