package log

import (
	"github.com/go-logr/logr"
)

// Logger wraps logr.Logger to provide INFO, WARN, and ERROR levels.
// In logr, verbosity works inversely: V(0) is most important, higher V = less important.
// We map: ERROR = always shown, WARN = V(0), INFO = V(0)
type Logger struct {
	base logr.Logger
}

// New creates a new Logger wrapping a logr.Logger
func New(base logr.Logger) *Logger {
	return &Logger{base: base}
}

// Info logs at INFO level (V=0)
func (l *Logger) Info(msg string, keysAndValues ...any) {
	l.base.Info(msg, keysAndValues...)
}

// Warn logs at WARN level with a "level"="WARN" key
func (l *Logger) Warn(msg string, keysAndValues ...any) {
	kvs := append([]any{"level", "WARN"}, keysAndValues...)
	l.base.Info(msg, kvs...)
}

// Error logs at ERROR level
func (l *Logger) Error(err error, msg string, keysAndValues ...any) {
	l.base.Error(err, msg, keysAndValues...)
}

// WithValues returns a new Logger with additional key-value pairs
func (l *Logger) WithValues(keysAndValues ...any) *Logger {
	return &Logger{base: l.base.WithValues(keysAndValues...)}
}

// WithName returns a new Logger with the specified name appended
func (l *Logger) WithName(name string) *Logger {
	return &Logger{base: l.base.WithName(name)}
}
