package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Writer writes tool-owned files atomically (FILE-2). It refuses to touch the
// user-owned files (ARCH-7).
type Writer struct{}

var userOwned = map[string]bool{"models.json": true, "strategies.json": true}

func checkNotUserOwned(path string) error {
	if userOwned[filepath.Base(path)] {
		return fmt.Errorf("refusing to write %s: the file is owned by the user", path)
	}
	return nil
}

// WriteAtomic writes data to a temp file in the same directory, then renames it.
func (Writer) WriteAtomic(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkNotUserOwned(path); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file for %s: %w", path, err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file for %s: %w", path, err)
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return fmt.Errorf("setting permissions on %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}
