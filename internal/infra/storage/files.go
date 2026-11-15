// Package storage implements file access for the application layer.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Files reads user files from disk. It has no write methods on purpose
// (ARCH-7, CFG-6).
type Files struct{}

// Read returns the file content. The error satisfies errors.Is(err, fs.ErrNotExist)
// when the file is missing.
func (Files) Read(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("reading %s: %w", path, fs.ErrNotExist)
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}

// Mode returns the permission bits of the file.
func (Files) Mode(ctx context.Context, path string) (fs.FileMode, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("checking permissions of %s: %w", path, err)
	}
	return info.Mode().Perm(), nil
}
