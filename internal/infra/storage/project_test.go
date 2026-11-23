package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSDD59_ProjectFilesWriteAtomicAndNeverUserConfig(t *testing.T) {
	dir := t.TempDir()
	p := ProjectFiles{}
	target := filepath.Join(dir, "a", "b", "file.go")
	if err := p.WriteAtomic(context.Background(), target, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "x" {
		t.Fatal("content")
	}
	if ents, _ := os.ReadDir(filepath.Dir(target)); len(ents) != 1 {
		t.Fatalf("no temp file may remain: %v", ents)
	}
	for _, name := range []string{"models.json", "strategies.json"} {
		path := filepath.Join(dir, name)
		if err := p.WriteAtomic(context.Background(), path, []byte("{}")); err == nil {
			t.Errorf("%s must never be written", name)
		}
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s exists", name)
		}
	}
	os.Chmod(target, 0o640)
	p.WriteAtomic(context.Background(), target, []byte("y"))
	if st, _ := os.Stat(target); st.Mode().Perm() != 0o640 {
		t.Fatalf("an existing mode must be kept, got %v", st.Mode())
	}
}
