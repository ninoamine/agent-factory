package storage

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ProjectFiles gives SDD access to the user's project dir. Writes are atomic
// (FILE-2, CONV-7) and never touch the user-owned config files (SDD-59).
type ProjectFiles struct{}

func (ProjectFiles) Read(path string) ([]byte, error) { return os.ReadFile(path) }

// Head returns at most n leading bytes of a file.
func (ProjectFiles) Head(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	k, err := io.ReadFull(f, buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		err = nil
	}
	return buf[:k], err
}

func (ProjectFiles) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }
func (ProjectFiles) Lstat(path string) (fs.FileInfo, error)     { return os.Lstat(path) }
func (ProjectFiles) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (ProjectFiles) EvalSymlinks(path string) (string, error)   { return filepath.EvalSymlinks(path) }
func (ProjectFiles) MkdirAll(path string) error                 { return os.MkdirAll(path, 0o755) }

// WriteAtomic writes a temp file next to the target, then renames it. It keeps
// the mode of an existing file, else uses 0644. Parent dirs are created.
func (p ProjectFiles) WriteAtomic(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkNotUserOwned(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", path, err)
	}
	mode := fs.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file for %s: %w", path, err)
	}
	defer os.Remove(tmp.Name())
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
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return fmt.Errorf("setting permissions on %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}
