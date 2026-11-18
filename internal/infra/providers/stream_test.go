package providers_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ninoamine/agent-factory/internal/app/execution"
)

const anthropicStream = `data: {"type":"message_start","message":{"usage":{"input_tokens":12,"output_tokens":1}}}
data: {"type":"content_block_start","content_block":{"type":"text"}}
data: {"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"hmm"}}
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hel"}}
data: {"type":"ping"}
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"lo"}}
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}
data: {"type":"message_stop"}`

func TestPROV12EveryAdapterStreamsAndCancels(t *testing.T) {
	for _, ty := range []string{"anthropic", "openai", "ollama"} {
		t.Run("PROV-12 "+ty+" implements streaming and cancel", func(t *testing.T) {
			a, _ := adapter(t, opts{typ: ty})
			if _, ok := a.(execution.Streamer); !ok {
				t.Error("not a Streamer")
			}
			if _, ok := a.(execution.Canceller); !ok {
				t.Error("not a Canceller")
			}
		})
	}
}

func TestPROV13_14Anthropic(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { sse(w, `data: {"type":"message_stop"}`) })
	if _, _, err := run(t, opts{typ: "anthropic", baseURL: srv.URL + "/", apiKey: "sk-ant"}, "hi"); err != nil {
		t.Fatal(err)
	}
	r := srv.requests()[0]
	t.Run("PROV-13 path, method and headers", func(t *testing.T) {
		if r.Method != "POST" || r.Path != "/v1/messages" {
			t.Fatalf("%s %s", r.Method, r.Path)
		}
		if r.Header.Get("x-api-key") != "sk-ant" || r.Header.Get("anthropic-version") != "2023-06-01" ||
			r.Header.Get("content-type") != "application/json" {
			t.Fatalf("headers: %v", r.Header)
		}
		if r.Body["stream"] != true || r.Body["model"] != "p/x" {
			t.Fatalf("body: %v", r.Body)
		}
	})
	t.Run("PROV-14 max_tokens defaults to 8192", func(t *testing.T) {
		if r.Body["max_tokens"] != float64(8192) {
			t.Fatalf("got %v", r.Body["max_tokens"])
		}
	})
	t.Run("PROV-14 maxOutputTokens overrides", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { sse(w, `data: {"type":"message_stop"}`) })
		run(t, opts{typ: "anthropic", baseURL: srv.URL, maxTokens: intp(1000)}, "hi")
		if got := srv.requests()[0].Body["max_tokens"]; got != float64(1000) {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("PROV-13 no x-api-key header without apiKey", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { sse(w, `data: {"type":"message_stop"}`) })
		run(t, opts{typ: "anthropic", baseURL: srv.URL}, "hi")
		if _, has := srv.requests()[0].Header["X-Api-Key"]; has {
			t.Fatal("header sent without a key")
		}
	})
}

func TestPROV15OpenAI(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) { sse(w, "data: [DONE]") }
	t.Run("PROV-15 path, auth, stream options, no max_tokens", func(t *testing.T) {
		srv := newServer(t, ok)
		run(t, opts{typ: "openai", baseURL: srv.URL + "/v1/", apiKey: "sk-o"}, "hi")
		r := srv.requests()[0]
		if r.Method != "POST" || r.Path != "/v1/chat/completions" {
			t.Fatalf("%s %s", r.Method, r.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-o" {
			t.Fatalf("auth: %q", r.Header.Get("Authorization"))
		}
		so, _ := r.Body["stream_options"].(map[string]any)
		if so["include_usage"] != true || r.Body["stream"] != true {
			t.Fatalf("body: %v", r.Body)
		}
		if _, has := r.Body["max_tokens"]; has {
			t.Fatal("max_tokens sent while unset")
		}
	})
	t.Run("PROV-15 no Authorization without apiKey", func(t *testing.T) {
		srv := newServer(t, ok)
		run(t, opts{typ: "openai", baseURL: srv.URL}, "hi")
		if _, has := srv.requests()[0].Header["Authorization"]; has {
			t.Fatal("Authorization sent without a key")
		}
	})
	t.Run("PROV-9 maxOutputTokens is sent as max_tokens", func(t *testing.T) {
		srv := newServer(t, ok)
		run(t, opts{typ: "openai", baseURL: srv.URL, maxTokens: intp(77)}, "hi")
		if got := srv.requests()[0].Body["max_tokens"]; got != float64(77) {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("PROV-15 a rejected stream_options fails once and is not re-sent", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"message":"unknown field stream_options"}}`))
		})
		_, _, err := run(t, opts{typ: "openai", baseURL: srv.URL}, "hi")
		if kindOf(err) != execution.KindProviderFailure || !strings.Contains(err.Error(), "stream_options") {
			t.Fatalf("err = %v", err)
		}
		if len(srv.requests()) != 1 {
			t.Fatalf("%d requests", len(srv.requests()))
		}
	})
}

func TestPROV16Ollama(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) { nd(w, `{"done":true}`) }
	t.Run("PROV-16 path, body, no auth, no options", func(t *testing.T) {
		srv := newServer(t, ok)
		run(t, opts{typ: "ollama", baseURL: srv.URL}, "hi")
		r := srv.requests()[0]
		if r.Method != "POST" || r.Path != "/api/chat" || r.Body["stream"] != true {
			t.Fatalf("%s %s %v", r.Method, r.Path, r.Body)
		}
		if _, has := r.Header["Authorization"]; has {
			t.Fatal("Authorization sent without a key")
		}
		if _, has := r.Body["options"]; has {
			t.Fatal("options sent while maxOutputTokens unset")
		}
	})
	t.Run("PROV-16 num_predict from maxOutputTokens, bearer with a key", func(t *testing.T) {
		srv := newServer(t, ok)
		run(t, opts{typ: "ollama", baseURL: srv.URL, maxTokens: intp(50), apiKey: "k"}, "hi")
		r := srv.requests()[0]
		o, _ := r.Body["options"].(map[string]any)
		if o["num_predict"] != float64(50) || r.Header.Get("Authorization") != "Bearer k" {
			t.Fatalf("%v %v", r.Body, r.Header)
		}
	})
}

func TestPROV8ModelNameSent(t *testing.T) {
	for _, ty := range []string{"anthropic", "openai", "ollama"} {
		t.Run("PROV-8 "+ty+" sends the model field, else the id", func(t *testing.T) {
			srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
			run(t, opts{typ: ty, baseURL: srv.URL, model: str("anthropic/claude-x")}, "hi")
			run(t, opts{typ: ty, baseURL: srv.URL}, "hi")
			reqs := srv.requests()
			if reqs[0].Body["model"] != "anthropic/claude-x" || reqs[1].Body["model"] != "p/x" {
				t.Fatalf("%v / %v", reqs[0].Body["model"], reqs[1].Body["model"])
			}
		})
	}
}

func TestPROV17OneUserMessage(t *testing.T) {
	for _, ty := range []string{"anthropic", "openai", "ollama"} {
		t.Run("PROV-17 "+ty+" sends only the stripped prompt", func(t *testing.T) {
			srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
			run(t, opts{typ: ty, baseURL: srv.URL}, "write a haiku")
			b := srv.requests()[0].Body
			msgs, _ := b["messages"].([]any)
			if len(msgs) != 1 {
				t.Fatalf("messages: %v", msgs)
			}
			m := msgs[0].(map[string]any)
			if m["role"] != "user" || m["content"] != "write a haiku" {
				t.Fatalf("message: %v", m)
			}
			if _, has := b["system"]; has {
				t.Fatal("system prompt sent")
			}
		})
	}
}

func TestPROV18UserAgent(t *testing.T) {
	for _, ty := range []string{"anthropic", "openai", "ollama"} {
		t.Run("PROV-18 "+ty+" sends User-Agent with the version", func(t *testing.T) {
			srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
			run(t, opts{typ: ty, baseURL: srv.URL}, "hi")
			if got := srv.requests()[0].Header.Get("User-Agent"); got != "agent-factory/9.9.9" {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestPROV19KeyHandlingAndRedirects(t *testing.T) {
	t.Run("PROV-19 the key is never in the URL", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
		for _, ty := range []string{"anthropic", "openai", "ollama"} {
			run(t, opts{typ: ty, baseURL: srv.URL, apiKey: "sk-in-url"}, "hi")
		}
		for _, r := range srv.requests() {
			if strings.Contains(r.Path, "sk-in-url") {
				t.Fatalf("key in path %s", r.Path)
			}
		}
	})
	t.Run("PROV-19 a redirect is not followed and fails", func(t *testing.T) {
		other := newServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.URL+"/steal", http.StatusFound)
		})
		_, _, err := run(t, opts{typ: "openai", baseURL: srv.URL, apiKey: "sk-secret"}, "hi")
		if kindOf(err) != execution.KindProviderFailure || !strings.Contains(err.Error(), "HTTP 302") {
			t.Fatalf("err = %v", err)
		}
		if len(other.requests()) != 0 {
			t.Fatal("redirect target was contacted")
		}
	})
}

func TestPROV20_29Timeouts(t *testing.T) {
	t.Run("PROV-20 and 29 a stalled stream fails after the chunk was shown", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			sse(w, `data: {"choices":[{"delta":{"content":"partial"}}]}`)
			<-r.Context().Done()
		})
		start := time.Now()
		shown, _, err := run(t, opts{typ: "openai", baseURL: srv.URL, timeout: intp(1)}, "hi")
		if shown != "partial" {
			t.Fatalf("shown %q", shown)
		}
		if kindOf(err) != execution.KindProviderFailure || err.Error() != `provider_failure: no response from "p" for 1s` {
			t.Fatalf("err = %v", err)
		}
		if d := time.Since(start); d < 900*time.Millisecond || d > 3*time.Second {
			t.Fatalf("took %v", d)
		}
	})
	t.Run("PROV-20 no response headers within the timeout fails", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
		_, _, err := run(t, opts{typ: "ollama", baseURL: srv.URL, timeout: intp(1)}, "hi")
		if err == nil || !strings.Contains(err.Error(), `no response from "p" for 1s`) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("PROV-20 a slow but steady stream is not capped", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			for i := 0; i < 6; i++ {
				nd(w, `{"message":{"content":"x"}}`)
				time.Sleep(400 * time.Millisecond)
			}
			nd(w, `{"done":true}`)
		})
		shown, _, err := run(t, opts{typ: "ollama", baseURL: srv.URL, timeout: intp(1)}, "hi")
		if err != nil || shown != "xxxxxx" {
			t.Fatalf("shown %q err %v", shown, err)
		}
	})
	t.Run("PROV-20 the idle wait restarts once headers arrive", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(700 * time.Millisecond)
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			time.Sleep(700 * time.Millisecond)
			nd(w, `{"message":{"content":"x"}}`, `{"done":true}`)
		})
		shown, _, err := run(t, opts{typ: "ollama", baseURL: srv.URL, timeout: intp(1)}, "hi")
		if err != nil || shown != "x" {
			t.Fatalf("shown %q err %v", shown, err)
		}
	})
	t.Run("PROV-38 metadata uses the same timeout", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
		r := registry(opts{typ: "anthropic", baseURL: srv.URL, timeout: intp(1)})
		_, err := r.FetchMetadata(context.Background(), "p", "p/x")
		if err == nil || !strings.Contains(err.Error(), `no response from "p" for 1s`) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPROV22AnthropicStream(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		for _, l := range strings.Split(anthropicStream, "\n") {
			sse(w, l)
		}
	}
	t.Run("PROV-22 text deltas are chunks, thinking is ignored, usage 12/5/17", func(t *testing.T) {
		srv := newServer(t, handler)
		a, _ := adapter(t, opts{typ: "anthropic", baseURL: srv.URL})
		var chunks []string
		resp, err := a.(execution.Streamer).Stream(context.Background(), "p/x", "hi", func(s string) { chunks = append(chunks, s) })
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(chunks, "|") != "Hel|lo" || resp.Text != "Hello" {
			t.Fatalf("chunks %v text %q", chunks, resp.Text)
		}
		if n(resp.Usage.Input) != 12 || n(resp.Usage.Output) != 5 || n(resp.Usage.Total) != 17 {
			t.Fatalf("usage %d/%d/%d", n(resp.Usage.Input), n(resp.Usage.Output), n(resp.Usage.Total))
		}
	})
	t.Run("PROV-22 an error event ends the request as a failure", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			sse(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"a"}}`,
				`data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
		})
		shown, _, err := run(t, opts{typ: "anthropic", baseURL: srv.URL}, "hi")
		if kindOf(err) != execution.KindProviderFailure || !strings.Contains(err.Error(), "Overloaded") || shown != "a" {
			t.Fatalf("shown %q err %v", shown, err)
		}
	})
	t.Run("PROV-25 output only leaves total unavailable", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			sse(w, `data: {"type":"message_delta","usage":{"output_tokens":5}}`, `data: {"type":"message_stop"}`)
		})
		_, resp, _ := run(t, opts{typ: "anthropic", baseURL: srv.URL}, "hi")
		if resp.Usage.Input != nil || resp.Usage.Total != nil || n(resp.Usage.Output) != 5 {
			t.Fatalf("usage %+v", resp.Usage)
		}
	})
}

func TestPROV23OpenAIStream(t *testing.T) {
	t.Run("PROV-23 and 25 no usage chunk leaves usage unavailable", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			sse(w, `data: {"choices":[{"delta":{"role":"assistant","content":""}}]}`,
				`data: {"choices":[{"delta":{"content":"Hi"}}]}`,
				`data: {"choices":[{"delta":{"content":null}}]}`, "data: [DONE]")
		})
		shown, resp, err := run(t, opts{typ: "openai", baseURL: srv.URL}, "hi")
		if err != nil || shown != "Hi" {
			t.Fatalf("shown %q err %v", shown, err)
		}
		if resp.Usage.Input != nil || resp.Usage.Output != nil || resp.Usage.Total != nil {
			t.Fatalf("usage %+v", resp.Usage)
		}
	})
	t.Run("PROV-23 and 25 usage chunk 3/4/7", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			sse(w, `data: {"choices":[{"delta":{"content":"Hi"}}]}`,
				`data: {"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`, "data: [DONE]")
		})
		_, resp, _ := run(t, opts{typ: "openai", baseURL: srv.URL}, "hi")
		if n(resp.Usage.Input) != 3 || n(resp.Usage.Output) != 4 || n(resp.Usage.Total) != 7 {
			t.Fatalf("usage %+v", resp.Usage)
		}
	})
	t.Run("PROV-25 the provider's own total wins", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			sse(w, `data: {"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":9}}`, "data: [DONE]")
		})
		_, resp, _ := run(t, opts{typ: "openai", baseURL: srv.URL}, "hi")
		if n(resp.Usage.Total) != 9 {
			t.Fatalf("total %d", n(resp.Usage.Total))
		}
	})
	t.Run("PROV-25 total is input plus output when the provider gives none", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			sse(w, `data: {"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4}}`, "data: [DONE]")
		})
		_, resp, _ := run(t, opts{typ: "openai", baseURL: srv.URL}, "hi")
		if n(resp.Usage.Total) != 7 {
			t.Fatalf("total %d", n(resp.Usage.Total))
		}
	})
	t.Run("PROV-23 an error chunk ends the request as a failure", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			sse(w, `data: {"error":{"message":"quota exceeded"}}`)
		})
		_, _, err := run(t, opts{typ: "openai", baseURL: srv.URL}, "hi")
		if kindOf(err) != execution.KindProviderFailure || !strings.Contains(err.Error(), "quota exceeded") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPROV24OllamaStream(t *testing.T) {
	t.Run("PROV-24 NDJSON chunks and usage 8/2/10", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			nd(w, `{"message":{"content":"a"},"done":false}`, "", `{"message":{"content":"b"},"done":false}`,
				`{"message":{"content":""},"done":true,"prompt_eval_count":8,"eval_count":2}`)
		})
		shown, resp, err := run(t, opts{typ: "ollama", baseURL: srv.URL}, "hi")
		if err != nil || shown != "ab" || resp.Text != "ab" {
			t.Fatalf("shown %q err %v", shown, err)
		}
		if n(resp.Usage.Input) != 8 || n(resp.Usage.Output) != 2 || n(resp.Usage.Total) != 10 {
			t.Fatalf("usage %+v", resp.Usage)
		}
	})
	t.Run("PROV-24 an error line ends the request as a failure", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			nd(w, `{"message":{"content":"a"}}`, `{"error":"model runner crashed"}`)
		})
		shown, _, err := run(t, opts{typ: "ollama", baseURL: srv.URL}, "hi")
		if kindOf(err) != execution.KindProviderFailure || !strings.Contains(err.Error(), "model runner crashed") || shown != "a" {
			t.Fatalf("shown %q err %v", shown, err)
		}
	})
}

func TestPROV26IncompleteStreams(t *testing.T) {
	cases := map[string]struct {
		o    opts
		body func(w http.ResponseWriter)
	}{
		"anthropic ends before message_stop": {opts{typ: "anthropic"}, func(w http.ResponseWriter) {
			sse(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"so far"}}`)
		}},
		"openai ends before DONE": {opts{typ: "openai"}, func(w http.ResponseWriter) {
			sse(w, `data: {"choices":[{"delta":{"content":"so far"}}]}`)
		}},
		"ollama ends before done": {opts{typ: "ollama"}, func(w http.ResponseWriter) {
			nd(w, `{"message":{"content":"so far"}}`)
		}},
	}
	for name, c := range cases {
		t.Run("PROV-26 "+name, func(t *testing.T) {
			srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { c.body(w) })
			c.o.baseURL = srv.URL
			shown, _, err := run(t, c.o, "hi")
			if kindOf(err) != execution.KindProviderFailure || err.Error() != `provider_failure: "p" stream failed: ended early` {
				t.Fatalf("err = %v", err)
			}
			if shown != "so far" {
				t.Fatalf("shown %q", shown)
			}
		})
	}
	t.Run("PROV-26 an undecodable line is a failure", func(t *testing.T) {
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { sse(w, `data: {not json`) })
		_, _, err := run(t, opts{typ: "openai", baseURL: srv.URL}, "hi")
		if kindOf(err) != execution.KindProviderFailure || !strings.Contains(err.Error(), "stream failed") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPROV27Cancel(t *testing.T) {
	t.Run("PROV-27 cancelling closes the connection and Cancel returns nil", func(t *testing.T) {
		started := make(chan struct{})
		closed := make(chan struct{})
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			sse(w, `data: {"choices":[{"delta":{"content":"a"}}]}`)
			close(started)
			<-r.Context().Done()
			close(closed)
		})
		a, _ := adapter(t, opts{typ: "openai", baseURL: srv.URL})
		ctx, cancel := context.WithCancel(context.Background())
		errc := make(chan error, 1)
		go func() {
			_, err := a.(execution.Streamer).Stream(ctx, "p/x", "hi", func(string) {})
			errc <- err
		}()
		<-started
		cancel()
		err := <-errc
		if kindOf(err) != execution.KindInterrupted {
			t.Fatalf("err = %v", err)
		}
		if cerr := a.(execution.Canceller).Cancel(); cerr != nil {
			t.Fatalf("Cancel() = %v", cerr)
		}
		select {
		case <-closed:
		case <-time.After(3 * time.Second):
			t.Fatal("server side never saw the connection close")
		}
	})
	t.Run("PROV-27 Cancel stops a running request", func(t *testing.T) {
		started := make(chan struct{})
		srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			sse(w, `data: {"choices":[{"delta":{"content":"a"}}]}`)
			close(started)
			<-r.Context().Done()
		})
		a, _ := adapter(t, opts{typ: "openai", baseURL: srv.URL})
		errc := make(chan error, 1)
		go func() {
			_, err := a.(execution.Streamer).Stream(context.Background(), "p/x", "hi", func(string) {})
			errc <- err
		}()
		<-started
		if err := a.(execution.Canceller).Cancel(); err != nil {
			t.Fatalf("Cancel() = %v", err)
		}
		select {
		case err := <-errc:
			if err == nil {
				t.Fatal("request finished without error")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("request still running after Cancel")
		}
	})
}
