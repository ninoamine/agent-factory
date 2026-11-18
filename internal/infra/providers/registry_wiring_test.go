package providers_test

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/infra/providers"
)

func TestPROV10ConfigureReplacesAdapters(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) { nd(w, `{"done":true}`) }
	a := newServer(t, ok)
	b := newServer(t, ok)
	r := providers.NewRegistry()
	r.Configure(config(opts{typ: "ollama", baseURL: a.URL}))
	r.Configure(config(opts{typ: "ollama", baseURL: b.URL})) // like /refresh with an edited baseUrl
	ex, found := r.Executor("p")
	if !found {
		t.Fatal("no executor")
	}
	if _, err := ex.Execute(context.Background(), "p/x", "hi"); err != nil {
		t.Fatal(err)
	}
	t.Run("PROV-10 the next request reaches the new baseUrl only", func(t *testing.T) {
		if len(a.requests()) != 0 || len(b.requests()) != 1 {
			t.Fatalf("a=%d b=%d", len(a.requests()), len(b.requests()))
		}
	})
	t.Run("PROV-10 a removed provider has no adapter afterwards", func(t *testing.T) {
		r.Configure(configuration.Config{})
		if _, found := r.Executor("p"); found {
			t.Fatal("executor survived")
		}
		if _, err := r.FetchMetadata(context.Background(), "p", "p/x"); err == nil {
			t.Fatal("metadata adapter survived")
		}
	})
	t.Run("PROV-10 one execution and one metadata adapter per provider", func(t *testing.T) {
		cfg := configuration.Config{Models: &configuration.ModelsFile{Providers: map[string]configuration.ProviderEntry{
			"a": {Type: "anthropic"}, "o": {Type: "openai"}, "l": {Type: "ollama"},
		}}}
		r.Configure(cfg)
		for _, n := range []string{"a", "o", "l"} {
			if _, found := r.Executor(n); !found {
				t.Errorf("no executor for %s", n)
			}
		}
	})
}

func TestPROV11StdlibOnly(t *testing.T) {
	t.Run("PROV-11 adapter code imports only the standard library and this module", func(t *testing.T) {
		files, _ := filepath.Glob("*.go")
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			fh, err := os.Open(f)
			if err != nil {
				t.Fatal(err)
			}
			sc := bufio.NewScanner(fh)
			for sc.Scan() {
				l := strings.TrimSpace(sc.Text())
				if !strings.HasPrefix(l, `"`) {
					continue
				}
				path := strings.Trim(l, `"`)
				first, _, _ := strings.Cut(path, "/")
				if strings.Contains(first, ".") && !strings.HasPrefix(path, "github.com/ninoamine/agent-factory/") {
					t.Errorf("%s imports %s", f, path)
				}
			}
			fh.Close()
		}
	})
	t.Run("PROV-11 go.mod has no provider SDK", func(t *testing.T) {
		data, err := os.ReadFile("../../../go.mod")
		if err != nil {
			t.Fatal(err)
		}
		for _, sdk := range []string{"anthropic", "openai", "ollama", "genai", "langchain"} {
			if strings.Contains(strings.ToLower(string(data)), sdk) {
				t.Errorf("go.mod mentions %s", sdk)
			}
		}
	})
}
