package providers_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/infra/providers"
)

// captured is one request seen by a fake provider.
type captured struct {
	Method string
	Path   string
	Header http.Header
	Body   map[string]any
	Raw    string
}

// fakeServer records every request and answers with handler.
type fakeServer struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []captured
}

func newServer(t *testing.T, handler http.HandlerFunc) *fakeServer {
	t.Helper()
	fs := &fakeServer{}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		c := captured{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Raw: string(raw)}
		_ = json.Unmarshal(raw, &c.Body)
		fs.mu.Lock()
		fs.reqs = append(fs.reqs, c)
		fs.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(fs.Close)
	return fs
}

func (f *fakeServer) requests() []captured {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]captured(nil), f.reqs...)
}

// opts configures provider "p" and model "p/x".
type opts struct {
	typ       string
	baseURL   string
	apiKey    string
	timeout   *int
	model     *string
	maxTokens *int
}

func intp(n int) *int { return &n }

func str(s string) *string { return &s }

func config(o opts) configuration.Config {
	return configuration.Config{Models: &configuration.ModelsFile{
		Providers: map[string]configuration.ProviderEntry{
			"p": {Type: o.typ, BaseURL: o.baseURL, APIKey: o.apiKey, TimeoutSeconds: o.timeout},
		},
		Models: []configuration.ModelEntry{{ID: "p/x", Provider: "p", Model: o.model, MaxOutputTokens: o.maxTokens}},
	}}
}

func registry(o opts) *providers.Registry {
	r := providers.NewRegistry()
	r.Version = "9.9.9"
	r.Configure(config(o))
	return r
}

// adapter returns the execution adapter of provider "p".
func adapter(t *testing.T, o opts) (execution.Adapter, *providers.Registry) {
	t.Helper()
	r := registry(o)
	a, ok := r.Executor("p")
	if !ok {
		t.Fatal("no executor for p")
	}
	return a, r
}

// run streams one prompt and returns what was shown and the result.
func run(t *testing.T, o opts, prompt string) (string, execution.Response, error) {
	t.Helper()
	a, _ := adapter(t, o)
	var shown string
	var chunks []string
	resp, err := a.(execution.Streamer).Stream(context.Background(), "p/x", prompt, func(s string) {
		shown += s
		chunks = append(chunks, s)
	})
	_ = chunks
	return shown, resp, err
}

func kindOf(err error) execution.ErrorKind {
	if e, ok := err.(*execution.Error); ok {
		return e.Kind
	}
	return ""
}

func sse(w http.ResponseWriter, lines ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, l := range lines {
		io.WriteString(w, l+"\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func nd(w http.ResponseWriter, lines ...string) {
	for _, l := range lines {
		io.WriteString(w, l+"\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func n(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}
