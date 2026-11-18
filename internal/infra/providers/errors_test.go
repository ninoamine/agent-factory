package providers_test

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/execution"
)

func TestPROV28StatusMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		hdr    map[string]string
		body   string
		kind   execution.ErrorKind
		want   []string
	}{
		{"401", 401, nil, `{"error":{"message":"bad key"}}`, execution.KindAuth,
			[]string{`provider "p" rejected the API key (HTTP 401)`, "providers.p.apiKey"}},
		{"403", 403, nil, ``, execution.KindAuth, []string{"(HTTP 403)"}},
		{"404", 404, nil, `{"error":{"message":"nope"}}`, execution.KindModelUnavailable, []string{`provider "p" has no model "p/x"`}},
		{"400 model not found", 400, nil, `{"error":{"message":"The model does not exist"}}`, execution.KindModelUnavailable, []string{"has no model"}},
		{"429 with Retry-After", 429, map[string]string{"Retry-After": "20"}, `{}`, execution.KindProviderFailure,
			[]string{`rate limited by "p" (HTTP 429)`, "retry after 20"}},
		{"429 without Retry-After", 429, nil, ``, execution.KindProviderFailure, []string{`rate limited by "p" (HTTP 429)`}},
		{"500", 500, nil, `{"error":{"message":"boom"}}`, execution.KindProviderFailure, []string{`"p" returned HTTP 500: boom`}},
		{"529 overloaded", 529, nil, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
			execution.KindProviderFailure, []string{`"p" returned HTTP 529: Overloaded`}},
		{"400 other", 400, nil, `plain text body`, execution.KindProviderFailure, []string{`"p" returned HTTP 400: plain text body`}},
		{"502 empty", 502, nil, ``, execution.KindProviderFailure, []string{`"p" returned HTTP 502`}},
	}
	for _, ty := range []string{"anthropic", "openai", "ollama"} {
		for _, c := range cases {
			t.Run("PROV-28 "+ty+" "+c.name+" (one request, no retry)", func(t *testing.T) {
				srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
					for k, v := range c.hdr {
						w.Header().Set(k, v)
					}
					w.WriteHeader(c.status)
					w.Write([]byte(c.body))
				})
				_, _, err := run(t, opts{typ: ty, baseURL: srv.URL}, "hi")
				if kindOf(err) != c.kind {
					t.Fatalf("kind %q, err %v", kindOf(err), err)
				}
				for _, w := range c.want {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("%q lacks %q", err.Error(), w)
					}
				}
				if len(srv.requests()) != 1 {
					t.Fatalf("%d requests, want 1", len(srv.requests()))
				}
			})
		}
	}
}

func TestPROV28Unreachable(t *testing.T) {
	// A port that was open and is now closed refuses connections.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	t.Run("PROV-28 connection refused names the provider and URL", func(t *testing.T) {
		_, _, err := run(t, opts{typ: "openai", baseURL: "http://" + addr + "/v1"}, "hi")
		if kindOf(err) != execution.KindProviderFailure || !strings.Contains(err.Error(), `cannot reach "p" at http://`+addr+`/v1`) {
			t.Fatalf("err = %v", err)
		}
		if strings.Contains(err.Error(), "is Ollama running?") {
			t.Fatal("Ollama hint on a non-Ollama provider")
		}
	})
	t.Run("PROV-28 Ollama adds a hint", func(t *testing.T) {
		_, _, err := run(t, opts{typ: "ollama", baseURL: "http://" + addr}, "hi")
		if err == nil || !strings.Contains(err.Error(), "is Ollama running?") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("PROV-28 the URL userinfo is not printed", func(t *testing.T) {
		_, _, err := run(t, opts{typ: "openai", baseURL: "http://user:hunter2@" + addr + "/v1"}, "hi")
		if err == nil || strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPROV28OllamaPullHint(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	_, _, err := run(t, opts{typ: "ollama", baseURL: srv.URL, model: str("qwen3:32b")}, "hi")
	if kindOf(err) != execution.KindModelUnavailable || !strings.Contains(err.Error(), "run: ollama pull qwen3:32b") {
		t.Fatalf("err = %v", err)
	}
}

func TestPROV28Cancelled(t *testing.T) {
	t.Run("PROV-28 a cancelled context is interrupted", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
		a, _ := adapter(t, opts{typ: "openai", baseURL: srv.URL})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := a.(execution.Streamer).Stream(ctx, "p/x", "hi", func(string) {})
		if kindOf(err) != execution.KindInterrupted {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPROV30ProviderMessage(t *testing.T) {
	t.Run("PROV-30 error field shapes of the three providers", func(t *testing.T) {
		for body, want := range map[string]string{
			`{"type":"error","error":{"type":"x","message":"anthropic says no"}}`: "anthropic says no",
			`{"error":{"message":"openai says no","code":"x"}}`:                   "openai says no",
			`{"error":"ollama says no"}`:                                          "ollama says no",
		} {
			srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500); w.Write([]byte(body)) })
			_, _, err := run(t, opts{typ: "openai", baseURL: srv.URL}, "hi")
			if !strings.HasSuffix(err.Error(), "HTTP 500: "+want) {
				t.Errorf("body %s: %v", body, err)
			}
		}
	})
	t.Run("PROV-30 a long message is cut to 300 characters and control characters are removed", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			w.Write([]byte("\x1b[31m" + strings.Repeat("é\n\t", 400)))
		})
		_, _, err := run(t, opts{typ: "openai", baseURL: srv.URL}, "hi")
		msg := err.Error()[strings.Index(err.Error(), "HTTP 500: ")+len("HTTP 500: "):]
		if n := len([]rune(msg)); n > 300 || n < 290 {
			t.Fatalf("message has %d characters", n)
		}
		for _, r := range err.Error() {
			if r < ' ' && r != ' ' {
				t.Fatalf("control character %q in %q", r, err.Error())
			}
		}
	})
}

func TestPROV32KeyNeverInErrors(t *testing.T) {
	t.Run("PROV-32 a key echoed by the provider is removed", func(t *testing.T) {
		const key = "sk-ECHOED-KEY-123"
		for _, ty := range []string{"anthropic", "openai", "ollama"} {
			srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(400)
				w.Write([]byte(`{"error":{"message":"invalid key ` + key + ` for request"}}`))
			})
			_, _, err := run(t, opts{typ: ty, baseURL: srv.URL, apiKey: key}, "hi")
			if err == nil || strings.Contains(err.Error(), key) {
				t.Errorf("%s: %v", ty, err)
			}
		}
	})
	t.Run("PROV-32 a key echoed in a stream error is removed", func(t *testing.T) {
		const key = "sk-ECHOED-KEY-123"
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			sse(w, `data: {"error":{"message":"bad `+key+`"}}`)
		})
		_, _, err := run(t, opts{typ: "openai", baseURL: srv.URL, apiKey: key}, "hi")
		if err == nil || strings.Contains(err.Error(), key) {
			t.Fatalf("err = %v", err)
		}
	})
}
