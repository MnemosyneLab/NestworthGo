// Package diagnostics owns opt-in, bounded local application logs.
package diagnostics

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const maxBytes = 5 * 1024 * 1024

// Log is a concurrency-safe writer shared by all handlers, including handlers
// derived with With. Disabled logging closes the file and writes nothing.
type Log struct {
	mu    sync.Mutex
	path  string
	file  *os.File
	size  int64
	level slog.LevelVar
}

func New(path string) *Log {
	l := &Log{path: path}
	l.level.Set(slog.Level(100))
	return l
}

func (l *Log) Logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(l, &slog.HandlerOptions{Level: &l.level}))
}

func ValidLevel(value string) bool {
	switch value {
	case "", "off", "error", "warn", "info", "debug":
		return true
	}
	return false
}

func (l *Log) Configure(value string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	level := slog.Level(100)
	switch value {
	case "", "off":
	case "error":
		level = slog.LevelError
	case "warn":
		level = slog.LevelWarn
	case "info":
		level = slog.LevelInfo
	case "debug":
		level = slog.LevelDebug
	default:
		return fmt.Errorf("unsupported log level")
	}
	if level == 100 {
		l.level.Set(level)
		if l.file != nil {
			err := l.file.Close()
			l.file = nil
			return err
		}
		return nil
	}
	if l.file == nil {
		if err := os.MkdirAll(filepath.Dir(l.path), 0700); err != nil {
			return err
		}
		file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return err
		}
		l.file, l.size = file, info.Size()
	}
	l.level.Set(level)
	return nil
}

func (l *Log) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return len(p), nil
	}
	if l.size+int64(len(p)) > maxBytes {
		if err := l.file.Close(); err != nil {
			return 0, err
		}
		l.file = nil
		// Current file plus two older files: at most approximately 15 MiB.
		if err := os.Rename(l.path+".1", l.path+".2"); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		if err := os.Rename(l.path, l.path+".1"); err != nil {
			return 0, err
		}
		file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return 0, err
		}
		l.file, l.size = file, 0
	}
	n, err := l.file.Write(p)
	l.size += int64(n)
	return n, err
}

func (l *Log) Close() error { return l.Configure("off") }
