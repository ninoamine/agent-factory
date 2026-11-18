package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
	"github.com/ninoamine/agent-factory/internal/infra/storage"
)

func TestHistoryMutates(t *testing.T) { // TUI-15 relies on this
	b := &backend{}
	for args, want := range map[string]bool{
		"": false, "delete abc": true, "clear": true, "list": false, "  clear ": true,
	} {
		if got := b.HistoryMutates(args); got != want {
			t.Errorf("HistoryMutates(%q) = %v, want %v", args, got, want)
		}
	}
}

type fakeRegistry struct{ urls []string }

func (f *fakeRegistry) Configure(cfg configuration.Config) {
	u := "none"
	if cfg.Models != nil {
		u = cfg.Models.Providers["p"].BaseURL
	}
	f.urls = append(f.urls, u)
}

func modelsWith(url string) string {
	return `{"providers":{"p":{"type":"ollama","baseUrl":"` + url + `"}},"models":[{"id":"p/x","provider":"p"}]}`
}

func TestPROV10BackendConfiguresAdapters(t *testing.T) {
	dir := t.TempDir()
	put := func(content string) {
		if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	put(modelsWith("http://a:1"))
	reg := &fakeRegistry{}
	files := storage.Files{}
	b := &backend{dir: dir, reader: files, registry: reg,
		metadata: metadata.Service{Reader: files, Writer: storage.Writer{}}}
	if _, err := b.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Run("PROV-10 startup builds adapters from the loaded configuration", func(t *testing.T) {
		if len(reg.urls) != 1 || reg.urls[0] != "http://a:1" {
			t.Fatalf("calls %v", reg.urls)
		}
	})
	t.Run("PROV-10 a successful refresh rebuilds adapters from the new configuration", func(t *testing.T) {
		put(modelsWith("http://b:2"))
		out, err := b.Refresh(context.Background())
		if err != nil || !out.Applied {
			t.Fatalf("%+v %v", out, err)
		}
		if got := reg.urls[len(reg.urls)-1]; got != "http://b:2" {
			t.Fatalf("calls %v", reg.urls)
		}
	})
	t.Run("PROV-10 a refresh that fails validation keeps the current adapters", func(t *testing.T) {
		before := len(reg.urls)
		put(`{"providers":{"p":{"apiKey":"x"}},"models":[]}`) // no type
		out, _ := b.Refresh(context.Background())
		if out.Applied || len(reg.urls) != before {
			t.Fatalf("applied=%v calls %v", out.Applied, reg.urls)
		}
	})
}
