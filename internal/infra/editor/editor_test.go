package editor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func script(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fake-editor.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSDD50_TempFileOwnerOnlyAndDeleted(t *testing.T) {
	out := filepath.Join(t.TempDir(), "mode.txt")
	cmd := script(t, `stat -c %a "$1" > "`+out+`"; echo "$1" >> "`+out+`"; echo edited > "$1"`)
	suspended, resumed := 0, 0
	e := Editor{Suspend: func() error { suspended++; return nil }, Resume: func() error { resumed++; return nil }}
	got, err := e.Edit(context.Background(), cmd, "original")
	if err != nil || strings.TrimSpace(got) != "edited" {
		t.Fatalf("%q %v", got, err)
	}
	b, _ := os.ReadFile(out)
	lines := strings.Fields(string(b))
	if lines[0] != "600" {
		t.Fatalf("temp file mode = %s, want 600", lines[0])
	}
	if _, err := os.Stat(lines[1]); err == nil {
		t.Fatal("temp file must be deleted after the editor exits")
	}
	if suspended != 1 || resumed != 1 {
		t.Fatalf("TUI-26: the screen must be suspended and restored: %d %d", suspended, resumed)
	}
}

func TestSDD50_NonZeroExitIsAnError(t *testing.T) {
	if _, err := (Editor{}).Edit(context.Background(), script(t, "exit 3"), "x"); err == nil {
		t.Fatal("a non-zero exit must be reported")
	}
	if _, err := (Editor{}).Edit(context.Background(), "  ", "x"); err == nil {
		t.Fatal("an empty command must be an error")
	}
}

func TestSDD50_EditorWithArguments(t *testing.T) {
	cmd := script(t, `echo "$1:$2" > "$2"`)
	got, err := (Editor{}).Edit(context.Background(), cmd+" -w", "x")
	if err != nil || !strings.HasPrefix(got, "-w:") {
		t.Fatalf("%q %v", got, err)
	}
}
