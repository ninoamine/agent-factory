package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/history"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
	"github.com/ninoamine/agent-factory/internal/infra/storage"
)

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

func TestBackendHistoryDelegates(t *testing.T) {
	t.Run("HIST-6 backend passes /history to the history service", func(t *testing.T) {
		dir := t.TempDir()
		b := &backend{history: &history.Service{Store: storage.HistoryFile{Path: filepath.Join(dir, "history.jsonl"), Writer: storage.Writer{}}}}
		if !b.HistoryMutates("clear") || b.HistoryMutates("") {
			t.Fatal("HistoryMutates must follow the history rules")
		}
		r, err := b.History(context.Background(), "")
		if err != nil || len(r.Lines) != 1 || r.Lines[0] != "history is empty" {
			t.Fatalf("got %+v, %v", r, err)
		}
		if r, _ := b.History(context.Background(), "clear"); r.Confirm == nil {
			t.Fatal("clear must ask for confirmation")
		}
	})
}
