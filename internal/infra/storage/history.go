package storage

import (
	"context"
	"fmt"
	"os"
)

// HistoryFile is the file store of history.jsonl. Appends add one line;
// rewrites are atomic (FILE-2, HIST-7).
type HistoryFile struct {
	Path   string
	Writer Writer
}

// Read returns the file content (errors.Is fs.ErrNotExist when absent).
func (h HistoryFile) Read(ctx context.Context) ([]byte, error) {
	return Files{}.Read(ctx, h.Path)
}

// Append adds data at the end, creating the file silently if needed (HIST-9).
// A missing final newline (left by a broken write) is repaired first so the
// new line does not merge into the corrupted one.
func (h HistoryFile) Append(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkNotUserOwned(h.Path); err != nil {
		return err
	}
	f, err := os.OpenFile(h.Path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("opening %s: %w", h.Path, err)
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Size() > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, info.Size()-1); err == nil && last[0] != '\n' {
			data = append([]byte{'\n'}, data...)
		}
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("appending to %s: %w", h.Path, err)
	}
	return f.Close()
}

// Rewrite replaces the file atomically.
func (h HistoryFile) Rewrite(ctx context.Context, data []byte) error {
	return h.Writer.WriteAtomic(ctx, h.Path, data)
}
