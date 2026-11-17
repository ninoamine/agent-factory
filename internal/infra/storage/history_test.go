package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryFile_HIST9_CreatesOnFirstAppend(t *testing.T) {
	t.Setenv("AGENT_FACTORY_HOME", t.TempDir())
	h := HistoryFile{Path: filepath.Join(os.Getenv("AGENT_FACTORY_HOME"), "history.jsonl")}
	if _, err := h.Read(context.Background()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("HIST-9 absent file: %v", err)
	}
	if err := h.Append(context.Background(), []byte("a\n")); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(h.Path); string(data) != "a\n" {
		t.Errorf("HIST-9 got %q", data)
	}
}

func TestHistoryFile_HIST10_RepairsMissingNewline(t *testing.T) {
	h := HistoryFile{Path: filepath.Join(t.TempDir(), "history.jsonl")}
	os.WriteFile(h.Path, []byte("broken"), 0o600)
	if err := h.Append(context.Background(), []byte("ok\n")); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(h.Path); string(data) != "broken\nok\n" {
		t.Errorf("HIST-10 got %q", data)
	}
}

func TestHistoryFile_HIST7_RewriteAtomicNoTempLeft(t *testing.T) {
	h := HistoryFile{Path: filepath.Join(t.TempDir(), "history.jsonl")}
	_ = h.Append(context.Background(), []byte("a\nb\n"))
	if err := h.Rewrite(context.Background(), []byte("b\n")); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(h.Path); string(data) != "b\n" {
		t.Errorf("HIST-7 got %q", data)
	}
	if m, _ := filepath.Glob(h.Path + ".tmp-*"); len(m) != 0 {
		t.Errorf("HIST-7 temp left: %v", m)
	}
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.Rewrite(cctx, []byte("x\n")); err == nil {
		t.Fatal("expected error")
	}
	if data, _ := os.ReadFile(h.Path); string(data) != "b\n" {
		t.Errorf("HIST-7 aborted rewrite changed file: %q", data)
	}
}

func TestHistoryFile_ARCH7_TEST4_NeverWritesUserFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"models.json", "strategies.json"} {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte("keep"), 0o600)
		h := HistoryFile{Path: p}
		e1 := h.Append(context.Background(), []byte("x\n"))
		e2 := h.Rewrite(context.Background(), []byte("x\n"))
		if e1 == nil || e2 == nil || !strings.Contains(e1.Error(), "owned by the user") {
			t.Errorf("ARCH-7 %s: append=%v rewrite=%v", name, e1, e2)
		}
		if data, _ := os.ReadFile(p); string(data) != "keep" {
			t.Errorf("ARCH-7 %s modified: %q", name, data)
		}
	}
}
