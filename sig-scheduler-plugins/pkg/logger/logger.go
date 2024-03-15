package logger

// A small debug logger to write to file instead of klog
// I don't know where to close, so I'm opening and appending each time
// It's a bad design, but will work for debugging.

import (
	"fmt"
	"log"
	"os"
)

const (
	LevelNone = iota
	LevelInfo
	LevelWarning
	LevelError
	LevelVerbose
	LevelDebug
)

// TODO try saving state here when we can close
type DebugLogger struct {
	level    int
	Filename string
	handle   *os.File
}

func NewDebugLogger(level int, filename string) *DebugLogger {
	return &DebugLogger{
		level:    LevelNone,
		Filename: filename,
	}
}

func (l *DebugLogger) Start() (*log.Logger, error) {
	f, err := os.OpenFile(l.Filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, os.ModePerm)
	if err != nil {
		return nil, err
	}
	logger := log.New(f, "", 0)
	l.handle = f
	return logger, nil
}
func (l *DebugLogger) Stop() error {
	if l.handle != nil {
		return l.handle.Close()
	}
	return nil
}

// Logging functions you should use!
func (l *DebugLogger) Info(message ...any) error {
	return l.log(LevelInfo, "   INFO: ", message...)
}
func (l *DebugLogger) Error(message ...any) error {
	return l.log(LevelError, "  ERROR: ", message...)
}
func (l *DebugLogger) Debug(message ...any) error {
	return l.log(LevelDebug, "  DEBUG: ", message...)
}
func (l *DebugLogger) Verbose(message ...any) error {
	return l.log(LevelVerbose, "VERBOSE: ", message...)
}
func (l *DebugLogger) Warning(message ...any) error {
	return l.log(LevelWarning, "WARNING: ", message...)
}

// log is the shared class function for actually printing to the log
func (l *DebugLogger) log(level int, prefix string, message ...any) error {
	logger, err := l.Start()
	if err != nil {
		return err
	}
	// Assume the prolog (to be formatted) is at index 0
	prolog := message[0].(string)
	if prefix != "" {
		prolog = prefix + " " + prolog
	}
	rest := message[1:]

	//	msg := fmt.Sprintf(message...)
	fmt.Printf("Compariing level %d >= %d\n", level, l.level)
	if level >= l.level {
		logger.Printf(prolog, rest...)
	}
	return l.Stop()
}
