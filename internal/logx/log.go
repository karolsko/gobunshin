// Package logx prints the release log.
package logx

import (
	"fmt"
	"io"
	"sync"
)

// Logger writes step lines. It is safe for concurrent builds.
type Logger struct {
	mu      sync.Mutex
	w       io.Writer
	verbose bool
}

// New writes to w. Verbose lines are kept only when verbose is set.
func New(w io.Writer, verbose bool) *Logger {
	if w == nil {
		w = io.Discard
	}
	return &Logger{w: w, verbose: verbose}
}

// Step prints a top-level line.
func (l *Logger) Step(format string, args ...any) {
	l.write("• " + fmt.Sprintf(format, args...))
}

// Sub prints a line nested under the current step.
func (l *Logger) Sub(format string, args ...any) {
	l.write("  • " + fmt.Sprintf(format, args...))
}

// Verbose prints a line only when verbose logging is on.
func (l *Logger) Verbose(format string, args ...any) {
	if !l.verbose {
		return
	}
	l.write("    " + fmt.Sprintf(format, args...))
}

func (l *Logger) write(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintln(l.w, s)
}
