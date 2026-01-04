// Package logging provides structured logging with level filtering.
//
// The Logger supports DEBUG, INFO, WARN, and ERROR levels.
// Messages below the configured level are silently discarded.
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// Level represents a log level
type Level int

const (
	// LevelDebug is the debug log level
	LevelDebug Level = iota
	// LevelInfo is the info log level
	LevelInfo
	// LevelWarn is the warn log level
	LevelWarn
	// LevelError is the error log level
	LevelError
)

// String returns the string representation of a log level
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses a log level string into a Level.
// Returns LevelInfo if the string is not recognized.
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// Logger provides leveled logging
type Logger struct {
	level  Level
	logger *log.Logger
}

// New creates a new Logger with the specified level and output writer.
// If output is nil, os.Stderr is used.
func New(level Level, output io.Writer) *Logger {
	if output == nil {
		output = os.Stderr
	}

	return &Logger{
		level:  level,
		logger: log.New(output, "", log.LstdFlags),
	}
}

// NewFromString creates a new Logger from a level string.
// If output is nil, os.Stderr is used.
func NewFromString(levelStr string, output io.Writer) *Logger {
	return New(ParseLevel(levelStr), output)
}

// Debug logs a debug message
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.level <= LevelDebug {
		l.log(LevelDebug, format, v...)
	}
}

// Info logs an info message
func (l *Logger) Info(format string, v ...interface{}) {
	if l.level <= LevelInfo {
		l.log(LevelInfo, format, v...)
	}
}

// Warn logs a warning message
func (l *Logger) Warn(format string, v ...interface{}) {
	if l.level <= LevelWarn {
		l.log(LevelWarn, format, v...)
	}
}

// Error logs an error message
func (l *Logger) Error(format string, v ...interface{}) {
	if l.level <= LevelError {
		l.log(LevelError, format, v...)
	}
}

// log writes a log message with the given level
func (l *Logger) log(level Level, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.logger.Printf("[%s] %s", level.String(), msg)
}

// SetLevel changes the logger's level
func (l *Logger) SetLevel(level Level) {
	l.level = level
}

// GetLevel returns the logger's current level
func (l *Logger) GetLevel() Level {
	return l.level
}
