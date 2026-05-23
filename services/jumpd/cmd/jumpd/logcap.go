package main

import (
	"os"
	"strings"
	"sync"
)

type cappedLogWriter struct {
	mu      sync.Mutex
	path    string
	max     int
	lines   []string
	partial string
}

func newCappedLogWriter(path string, maxLines int) *cappedLogWriter {
	return &cappedLogWriter{path: path, max: maxLines}
}

func (w *cappedLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.partial += string(p)
	for {
		idx := strings.IndexByte(w.partial, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(w.partial[:idx], "\r")
		w.partial = w.partial[idx+1:]
		w.lines = append(w.lines, line)
		if len(w.lines) > w.max {
			w.lines = append([]string(nil), w.lines[len(w.lines)-w.max:]...)
		}
	}

	return len(p), w.flushLocked()
}

func (w *cappedLogWriter) flushLocked() error {
	content := strings.Join(w.lines, "\n")
	if content != "" {
		content += "\n"
	}
	if w.partial != "" {
		content += w.partial
	}
	return os.WriteFile(w.path, []byte(content), 0o600)
}
