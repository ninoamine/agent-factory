package storage

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestReadMissingReportsNotExist(t *testing.T) {
	_, err := Files{}.Read(context.Background(), filepath.Join(t.TempDir(), "models.json"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file: got %v", err)
	}
}

func TestReadContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Files{}).Read(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Fatalf("CONV-3 cancelled context: got %v", err)
	}
}
