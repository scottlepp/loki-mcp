package utils

import (
	"fmt"
	"log"
	"os"
	"time"
)

// Logger provides a simple logging interface with levels
type Logger struct {
	prefix string
	logger *log.Logger
}

// NewLogger creates a new logger with the given prefix
func NewLogger(prefix string) *Logger {
	return &Logger{
		prefix: prefix,
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
}

// Info logs an info message
func (l *Logger) Info(format string, v ...any) {
	l.log("INFO", format, v...)
}

// Error logs an error message
func (l *Logger) Error(format string, v ...any) {
	l.log("ERROR", format, v...)
}

// Debug logs a debug message
func (l *Logger) Debug(format string, v ...any) {
	l.log("DEBUG", format, v...)
}

func (l *Logger) log(level, format string, v ...any) {
	message := fmt.Sprintf(format, v...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	l.logger.Printf("[%s] [%s] %s: %s", timestamp, level, l.prefix, message)
}
