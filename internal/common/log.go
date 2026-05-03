package common

import (
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
)

var (
	verbose atomic.Bool
	logMu   sync.Mutex
)

// SetVerbose sets the global verbose flag.
func SetVerbose(v bool) {
	verbose.Store(v)
}

func logf(level, format string, args ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	msg := fmt.Sprintf(format, args...)
	log.Printf("[%s] %s", level, msg)
}

// Info logs an info-level message.
func Info(format string, args ...any) {
	logf("INFO", format, args...)
}

// Warn logs a warning-level message.
func Warn(format string, args ...any) {
	logf("WARN", format, args...)
}

// Error logs an error-level message.
func Error(format string, args ...any) {
	logf("ERROR", format, args...)
}

// Debug logs a debug-level message (only when verbose).
func Debug(format string, args ...any) {
	if verbose.Load() {
		logf("DEBUG", format, args...)
	}
}

// Fatal logs and exits.
func Fatal(format string, args ...any) {
	logf("FATAL", format, args...)
	os.Exit(1)
}
