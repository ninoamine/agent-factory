package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ninoamine/agent-factory/internal/app/execution"
)

const (
	defaultTimeout   = 60 * time.Second // PROV-20
	maxErrorBody     = 64 << 10         // PROV-31
	maxProviderMsg   = 300              // PROV-30
	maxStreamLine    = 1 << 20          // longest accepted stream line
	defaultMaxTokens = 8192             // PROV-14
)

// modelInfo is what an adapter needs to know about one configured model.
type modelInfo struct {
	name      string // exact name sent to the API (PROV-8)
	maxTokens *int   // PROV-9
}

// base holds what the three adapters share: where to send requests, how to
// authenticate, timeouts, error mapping and cancel (PROV-19 to 21, 27 to 32).
type base struct {
	provider string
	baseURL  string
	apiKey   string
	timeout  time.Duration
	version  string
	models   map[string]modelInfo // by model id
	client   *http.Client

	reachHint string // advice appended when the provider cannot be reached
	pullHint  bool   // model_unavailable advises "ollama pull" (Ollama only)

	mu     sync.Mutex
	active map[*request]struct{}
}

func newBase(provider, baseURL, apiKey string, timeout time.Duration, version string, models map[string]modelInfo, rt http.RoundTripper) *base {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &base{
		provider: provider, baseURL: baseURL, apiKey: apiKey, timeout: timeout, version: version, models: models,
		client: &http.Client{
			Transport: rt,
			// A redirect could carry the key to another host (PROV-19): never follow.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		active: map[*request]struct{}{},
	}
}

// info returns the model entry; an id that is not configured falls back to itself.
func (b *base) info(modelID string) modelInfo {
	if m, ok := b.models[modelID]; ok {
		return m
	}
	return modelInfo{name: modelID}
}

var errIdle = errors.New("idle timeout")

// request is one HTTP exchange guarded by an idle timer (PROV-20): the timer
// is armed at the start and re-armed whenever data arrives. When it fires the
// request context is cancelled with errIdle as the cause.
type request struct {
	b      *base
	parent context.Context
	ctx    context.Context
	cancel context.CancelCauseFunc
	timer  *time.Timer
	done   chan struct{} // closed once the connection is closed (PROV-27)
	resp   *http.Response
	body   io.Reader
}

// do sends the request and returns it with its headers received. The caller
// must call finish.
func (b *base) do(parent context.Context, method, path string, body any, setHeaders func(*http.Request)) (*request, error) {
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, &execution.Error{Kind: execution.KindProviderFailure, Err: fmt.Errorf("encoding request: %w", err)}
		}
		rd = bytes.NewReader(data)
	}
	ctx, cancel := context.WithCancelCause(parent)
	r := &request{b: b, parent: parent, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	r.timer = time.AfterFunc(b.timeout, func() { cancel(errIdle) })
	b.mu.Lock()
	b.active[r] = struct{}{}
	b.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, method, b.baseURL+path, rd)
	if err != nil {
		r.finish()
		return nil, &execution.Error{Kind: execution.KindProviderFailure, Err: fmt.Errorf("building request: %s", b.scrub(err.Error()))}
	}
	req.Header.Set("User-Agent", "agent-factory/"+b.version) // PROV-18
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	setHeaders(req)
	resp, err := b.client.Do(req)
	if err != nil {
		cerr := b.transportErr(r, err)
		r.finish()
		return nil, cerr
	}
	r.resp = resp
	r.timer.Reset(b.timeout) // headers arrived: the idle wait starts fresh (PROV-20)
	r.body = &idleReader{r: resp.Body, timer: r.timer, d: b.timeout}
	return r, nil
}

// finish closes the connection and stops the timer. Safe to call twice.
func (r *request) finish() {
	r.timer.Stop()
	if r.resp != nil {
		r.resp.Body.Close()
	}
	r.cancel(nil)
	r.b.mu.Lock()
	delete(r.b.active, r)
	r.b.mu.Unlock()
	select {
	case <-r.done:
	default:
		close(r.done)
	}
}

// idleReader re-arms the idle timer whenever data arrives (PROV-20).
type idleReader struct {
	r     io.Reader
	timer *time.Timer
	d     time.Duration
}

func (i *idleReader) Read(p []byte) (int, error) {
	n, err := i.r.Read(p)
	if n > 0 {
		i.timer.Reset(i.d)
	}
	return n, err
}

// Cancel closes the connections of running requests and waits briefly for
// them to close (EXEC-3, PROV-27).
func (b *base) Cancel() error {
	b.mu.Lock()
	reqs := make([]*request, 0, len(b.active))
	for r := range b.active {
		reqs = append(reqs, r)
	}
	b.mu.Unlock()
	for _, r := range reqs {
		r.cancel(context.Canceled)
	}
	deadline := time.After(2 * time.Second)
	for _, r := range reqs {
		select {
		case <-r.done:
		case <-deadline:
			return errors.New("the connection did not close in time")
		}
	}
	return nil
}

// fail builds a normalized error (PROV-28).
func fail(kind execution.ErrorKind, format string, args ...any) error {
	return &execution.Error{Kind: kind, Err: fmt.Errorf(format, args...)}
}

// scrub removes the API key from text (PROV-32).
func (b *base) scrub(s string) string {
	if b.apiKey != "" {
		s = strings.ReplaceAll(s, b.apiKey, "[redacted]")
	}
	return s
}

// transportErr maps a failure with no usable response (PROV-28, PROV-29).
func (b *base) transportErr(r *request, err error) error {
	if r.parent.Err() != nil {
		return &execution.Error{Kind: execution.KindInterrupted, Err: r.parent.Err()}
	}
	if errors.Is(context.Cause(r.ctx), errIdle) {
		return b.timeoutErr()
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	hint := ""
	if b.hint() != "" {
		hint = "; " + b.hint()
	}
	return fail(execution.KindProviderFailure, "cannot reach %q at %s: %s%s",
		b.provider, redactedURL(b.baseURL), b.scrub(oneLine(err.Error())), hint)
}

func (b *base) timeoutErr() error {
	return fail(execution.KindProviderFailure, "no response from %q for %ds", b.provider, int(b.timeout/time.Second))
}

// hint is extra advice for a provider that cannot be reached; set per adapter.
func (b *base) hint() string { return b.reachHint }

func redactedURL(s string) string {
	if u, err := url.Parse(s); err == nil {
		return u.Redacted()
	}
	return s
}

// streamErr maps an error met while reading a stream (PROV-26, PROV-29).
func (b *base) streamErr(r *request, err error) error {
	if r.parent.Err() != nil {
		return &execution.Error{Kind: execution.KindInterrupted, Err: r.parent.Err()}
	}
	if errors.Is(context.Cause(r.ctx), errIdle) {
		return b.timeoutErr()
	}
	return b.streamFailed(oneLine(err.Error()))
}

func (b *base) streamFailed(reason string) error {
	return fail(execution.KindProviderFailure, "%q stream failed: %s", b.provider, b.scrub(reason))
}

// statusErr maps a non-2xx response to a normalized error (PROV-28). The
// response body is read up to 64 KiB (PROV-31).
func (b *base) statusErr(r *request, model modelInfo) error {
	resp := r.resp
	raw := readLimited(r.body)
	if r.parent.Err() != nil {
		return &execution.Error{Kind: execution.KindInterrupted, Err: r.parent.Err()}
	}
	if errors.Is(context.Cause(r.ctx), errIdle) {
		return b.timeoutErr()
	}
	msg := b.scrub(providerMessage(raw))
	code := resp.StatusCode
	switch {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return fail(execution.KindAuth, "provider %q rejected the API key (HTTP %d); check providers.%s.apiKey in models.json",
			b.provider, code, b.provider)
	case code == http.StatusNotFound || modelMissing(msg):
		return fail(execution.KindModelUnavailable, "provider %q has no model %q%s", b.provider, model.name, b.modelHint(model.name))
	case code == http.StatusTooManyRequests:
		s := fmt.Sprintf("rate limited by %q (HTTP 429)", b.provider)
		if ra := oneLine(resp.Header.Get("Retry-After")); ra != "" {
			s += ", retry after " + cut(ra, 40)
		}
		return fail(execution.KindProviderFailure, "%s", s)
	}
	if msg == "" {
		return fail(execution.KindProviderFailure, "%q returned HTTP %d", b.provider, code)
	}
	return fail(execution.KindProviderFailure, "%q returned HTTP %d: %s", b.provider, code, msg)
}

// modelMissing reports a provider message saying the model does not exist
// (PROV-28), for servers that answer 400 instead of 404.
func modelMissing(msg string) bool {
	l := strings.ToLower(msg)
	return strings.Contains(l, "model") && (strings.Contains(l, "not found") ||
		strings.Contains(l, "does not exist") || strings.Contains(l, "not_found"))
}

func (b *base) modelHint(name string) string {
	if b.pullHint {
		return "; run: ollama pull " + name
	}
	return ""
}

// providerMessage extracts the error text from a response body (PROV-30):
// the error field when present, else the raw body. It is sanitized and cut.
func providerMessage(raw []byte) string {
	var env struct {
		Error json.RawMessage `json:"error"`
	}
	text := string(raw)
	if json.Unmarshal(raw, &env) == nil && len(env.Error) > 0 {
		var s string
		var obj struct {
			Message string `json:"message"`
		}
		switch {
		case json.Unmarshal(env.Error, &s) == nil && s != "":
			text = s
		case json.Unmarshal(env.Error, &obj) == nil && obj.Message != "":
			text = obj.Message
		}
	}
	return cut(oneLine(text), maxProviderMsg)
}

// oneLine removes control characters, ANSI escapes included, except space (PROV-30).
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// lines reads a stream line by line, up to maxStreamLine per line.
func lines(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxStreamLine)
	return sc
}

func ok2xx(code int) bool { return code >= 200 && code < 300 }

// endErr is the error for a stream that stopped without its end marker
// (PROV-26): a read error, a timeout, a cancel, or an early end.
func (b *base) endErr(r *request, scanErr error) error {
	if scanErr != nil {
		return b.streamErr(r, scanErr)
	}
	if r.parent.Err() != nil {
		return &execution.Error{Kind: execution.KindInterrupted, Err: r.parent.Err()}
	}
	return b.streamFailed("ended early")
}

// readLimited reads at most 64 KiB of an error body (PROV-31).
func readLimited(r io.Reader) []byte {
	raw, _ := io.ReadAll(io.LimitReader(r, maxErrorBody))
	return raw
}
