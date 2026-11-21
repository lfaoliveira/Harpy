// Package logx creates per-run, per-package log files.
package logx

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lfaoliveira/harpy/src/internal/paths"
)

// Run is one harpy invocation's log directory.
type Run struct {
	ID  string
	Dir string
}

// NewRun creates a new log directory under the platform state directory.
func NewRun() (*Run, error) {
	base, err := paths.LogsDir()
	if err != nil {
		return nil, err
	}
	return NewRunIn(base)
}

// NewRunIn creates a new log directory under base.
func NewRunIn(base string) (*Run, error) {
	id, err := newRunID()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	return &Run{ID: id, Dir: dir}, nil
}

// Create opens a package log file inside the run directory.
func (r *Run) Create(name string) (*os.File, error) {
	filename := SafeName(name)
	if !strings.HasSuffix(strings.ToLower(filename), ".log") {
		filename += ".log"
	}
	return os.Create(filepath.Join(r.Dir, filename))
}

// SafeName turns a package/manager identifier into a portable file name.
func SafeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._")
	if out == "" {
		return "package"
	}
	return out
}

func newRunID() (string, error) {
	var suffix [3]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(suffix[:]), nil
}
