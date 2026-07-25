// Package app is the service layer between the detector engine and the desktop
// UI. It owns the detector's lifecycle, the notifier, and everything the
// frontend is allowed to see.
package app

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	logFileName = "kaa-rada.log"

	// retainedLogLines bounds what the UI can read back. A windowed app has no
	// terminal, so the last few hundred lines are the only diagnostics a user
	// can reach without opening a file manager.
	retainedLogLines = 300
)

// Log writes structured records to a file and keeps the most recent lines in
// memory for the UI.
type Log struct {
	*slog.Logger

	file   *os.File
	recent *recentLines
}

// OpenLog starts logging into the configuration directory. Records are appended
// so restarting the app does not discard the evidence of why it last failed.
func OpenLog(directory string) (*Log, error) {
	path := filepath.Join(directory, logFileName)

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening log %s: %w", path, err)
	}

	recent := &recentLines{limit: retainedLogLines}
	handler := slog.NewTextHandler(io.MultiWriter(file, recent), &slog.HandlerOptions{Level: slog.LevelInfo})

	return &Log{
		Logger: slog.New(handler),
		file:   file,
		recent: recent,
	}, nil
}

// Path locates the log file, so the UI can tell the user where to find it.
func (l *Log) Path() string { return l.file.Name() }

// Recent returns the retained lines, oldest first.
func (l *Log) Recent() []string { return l.recent.lines() }

func (l *Log) Close() error { return l.file.Close() }

// recentLines is an io.Writer that keeps the last few lines written to it.
type recentLines struct {
	limit int

	mutex sync.Mutex
	held  []string
}

func (r *recentLines) Write(record []byte) (int, error) {
	// slog hands over one complete record per call, ending in a newline.
	line := strings.TrimRight(string(record), "\n")

	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.held = append(r.held, line)
	if len(r.held) > r.limit {
		r.held = r.held[len(r.held)-r.limit:]
	}

	return len(record), nil
}

func (r *recentLines) lines() []string {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return append([]string(nil), r.held...)
}
