package providers_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
	"github.com/ninoamine/agent-factory/internal/infra/providers"
)

func fetch(t *testing.T, o opts) (metadata.APIMetadata, error) {
	t.Helper()
	return registry(o).FetchMetadata(context.Background(), "p", "p/x")
}

func isEmpty(m metadata.APIMetadata) bool {
	return len(m.Capabilities) == 0 && m.Speed == "" && m.Pricing == nil
}

func TestPROV33AnthropicMetadata(t *testing.T) {
	t.Run("PROV-33 and 37 existing model succeeds with empty metadata", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"id":"claude"}`)) })
		m, err := fetch(t, opts{typ: "anthropic", baseURL: srv.URL, apiKey: "k", model: str("claude-x/y")})
		if err != nil || !isEmpty(m) {
			t.Fatalf("%+v %v", m, err)
		}
		r := srv.requests()[0]
		if r.Method != "GET" || r.Path != "/v1/models/claude-x/y" && r.Path != "/v1/models/claude-x%2Fy" {
			t.Fatalf("%s %s", r.Method, r.Path)
		}
		if r.Header.Get("x-api-key") != "k" || r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Fatalf("headers %v", r.Header)
		}
	})
	t.Run("PROV-33 404 is a failure naming the model", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
		_, err := fetch(t, opts{typ: "anthropic", baseURL: srv.URL})
		if kindOf(err) != execution.KindModelUnavailable || !strings.Contains(err.Error(), `no model "p/x"`) {
			t.Fatalf("err = %v", err)
		}
	})
}

const orList = `{"data":[
 {"id":"a/priced","pricing":{"prompt":"0.000003","completion":"0.000015"}},
 {"id":"a/free","pricing":{"prompt":"0","completion":"0"}},
 {"id":"a/none"},
 {"id":"a/bad","pricing":{"prompt":"-1","completion":"0.1"}},
 {"id":"a/numeric","pricing":{"prompt":0.1,"completion":0.2}},
 {"id":"a/half","pricing":{"prompt":"0.1"}},
 {"id":"a/nan","pricing":{"prompt":"NaN","completion":"1"}}
]}`

func TestPROV34OpenAIMetadata(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(orList)) })
	get := func(model string) (metadata.APIMetadata, error) {
		return fetch(t, opts{typ: "openai", baseURL: srv.URL + "/v1", apiKey: "k", model: str(model)})
	}
	t.Run("PROV-34 per-token prices become USD per million", func(t *testing.T) {
		m, err := get("a/priced")
		if err != nil || m.Pricing == nil || *m.Pricing.InputPer1M != 3 || *m.Pricing.OutputPer1M != 15 {
			t.Fatalf("%+v %v", m.Pricing, err)
		}
		r := srv.requests()[0]
		if r.Method != "GET" || r.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer k" {
			t.Fatalf("%s %s %v", r.Method, r.Path, r.Header)
		}
	})
	t.Run("PROV-34 zero means free, not unknown", func(t *testing.T) {
		m, _ := get("a/free")
		if m.Pricing == nil || *m.Pricing.InputPer1M != 0 || *m.Pricing.OutputPer1M != 0 {
			t.Fatalf("%+v", m.Pricing)
		}
	})
	t.Run("PROV-34 and 37 missing, negative, numeric, partial or NaN pricing stays unknown", func(t *testing.T) {
		for _, id := range []string{"a/none", "a/bad", "a/numeric", "a/half", "a/nan"} {
			m, err := get(id)
			if err != nil || !isEmpty(m) {
				t.Errorf("%s: %+v %v", id, m, err)
			}
		}
	})
	t.Run("PROV-34 a model absent from the list is a failure", func(t *testing.T) {
		_, err := get("a/missing")
		if kindOf(err) != execution.KindModelUnavailable || !strings.Contains(err.Error(), `no model "a/missing"`) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("PROV-34 a failing list is a failure", func(t *testing.T) {
		bad := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
		if _, err := fetch(t, opts{typ: "openai", baseURL: bad.URL}); kindOf(err) != execution.KindProviderFailure {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("PROV-34 an invalid list is a failure", func(t *testing.T) {
		bad := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`<html>`)) })
		if _, err := fetch(t, opts{typ: "openai", baseURL: bad.URL}); kindOf(err) != execution.KindProviderFailure {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPROV35ListFetchedOnce(t *testing.T) {
	t.Run("PROV-35 two models on one provider cause one GET /models", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(orList)) })
		cfg := configuration.Config{Models: &configuration.ModelsFile{
			Providers: map[string]configuration.ProviderEntry{"p": {Type: "openai", BaseURL: srv.URL}},
			Models: []configuration.ModelEntry{
				{ID: "m1", Provider: "p", Model: str("a/priced")},
				{ID: "m2", Provider: "p", Model: str("a/free")},
			},
		}}
		r := providers.NewRegistry()
		r.Configure(cfg)
		for _, id := range []string{"m1", "m2"} {
			if _, err := r.FetchMetadata(context.Background(), "p", id); err != nil {
				t.Fatal(err)
			}
		}
		if got := len(srv.requests()); got != 1 {
			t.Fatalf("%d requests", got)
		}
		r.Configure(cfg) // a refresh fetches again
		r.FetchMetadata(context.Background(), "p", "m1")
		if got := len(srv.requests()); got != 2 {
			t.Fatalf("after refresh: %d requests", got)
		}
	})
}

func TestPROV36OllamaMetadata(t *testing.T) {
	t.Run("PROV-36 and 37 existing model succeeds with empty metadata", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"details":{}}`)) })
		m, err := fetch(t, opts{typ: "ollama", baseURL: srv.URL, model: str("qwen3:32b")})
		if err != nil || !isEmpty(m) {
			t.Fatalf("%+v %v", m, err)
		}
		r := srv.requests()[0]
		if r.Method != "POST" || r.Path != "/api/show" || r.Body["model"] != "qwen3:32b" {
			t.Fatalf("%s %s %v", r.Method, r.Path, r.Body)
		}
	})
	t.Run("PROV-36 404 is a failure with the pull hint", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
		_, err := fetch(t, opts{typ: "ollama", baseURL: srv.URL})
		if kindOf(err) != execution.KindModelUnavailable || !strings.Contains(err.Error(), "ollama pull") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPROV37NoCapabilitiesOrSpeed(t *testing.T) {
	t.Run("PROV-37 failures use the PROV-28 reasons", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) })
		_, err := fetch(t, opts{typ: "anthropic", baseURL: srv.URL})
		if kindOf(err) != execution.KindAuth || !strings.Contains(err.Error(), "rejected the API key") {
			t.Fatalf("err = %v", err)
		}
	})
}
