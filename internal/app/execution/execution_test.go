package execution

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/routing"
	"github.com/ninoamine/agent-factory/internal/domain"
)

const secretKey = "sk-secret-123"

type fakeAdapter struct {
	mu     sync.Mutex
	calls  []string // "model|prompt"
	resp   Response
	err    error
	chunks []string
}

func (f *fakeAdapter) Execute(_ context.Context, m, p string) (Response, error) {
	f.mu.Lock()
	f.calls = append(f.calls, m+"|"+p)
	f.mu.Unlock()
	return f.resp, f.err
}

type streamAdapter struct {
	fakeAdapter
	block   chan struct{} // if set, blocks after the first chunk until closed
	cancels int
	cancelE error
}

func (s *streamAdapter) Stream(ctx context.Context, m, p string, on func(string)) (Response, error) {
	s.mu.Lock()
	s.calls = append(s.calls, m+"|"+p)
	s.mu.Unlock()
	for i, c := range s.chunks {
		on(c)
		if i == 0 && s.block != nil {
			select {
			case <-ctx.Done():
				return Response{}, ctx.Err()
			case <-s.block:
			}
		}
	}
	return s.resp, s.err
}

type cancelStream struct{ streamAdapter }

func (c *cancelStream) Cancel() error { c.cancels++; return c.cancelE }

type resolver map[string]Adapter

func (r resolver) Executor(p string) (Adapter, bool) { a, ok := r[p]; return a, ok }

type out struct {
	mu                            sync.Mutex
	chunks, finals, notices, errs []string
}

func (o *out) Chunk(t string)  { o.mu.Lock(); o.chunks = append(o.chunks, t); o.mu.Unlock() }
func (o *out) Final(t string)  { o.finals = append(o.finals, t) }
func (o *out) Notice(t string) { o.notices = append(o.notices, t) }
func (o *out) Error(t string)  { o.errs = append(o.errs, t) }

type history struct{ recs []Record }

func (h *history) Record(_ context.Context, r Record) error { h.recs = append(h.recs, r); return nil }

func cfg() configuration.Config {
	return configuration.Config{Models: &configuration.ModelsFile{
		Providers: map[string]configuration.ProviderEntry{"p1": {APIKey: secretKey}, "p2": {APIKey: "k2"}},
		Models:    []configuration.ModelEntry{{ID: "m1", Provider: "p1"}, {ID: "m2", Provider: "p2"}},
	}}
}

func dec(model string) routing.Decision {
	return routing.Decision{Kind: routing.Routed, Command: domain.CmdCode, Model: domain.ModelRef(model), Prompt: "fix this"}
}

func ip(n int) *int { return &n }

func TestEXEC1_CallsOnlySelectedModelThroughAdapter(t *testing.T) {
	a1, a2 := &fakeAdapter{}, &fakeAdapter{}
	s := Service{Providers: resolver{"p1": a1, "p2": a2}}
	s.Run(context.Background(), cfg(), dec("m2"), &out{})
	if len(a1.calls) != 0 || len(a2.calls) != 1 || a2.calls[0] != "m2|fix this" {
		t.Fatalf("p1=%v p2=%v", a1.calls, a2.calls)
	}
}

func TestEXEC2_StreamsIncrementallyElseFinal(t *testing.T) {
	st := &streamAdapter{fakeAdapter: fakeAdapter{chunks: []string{"a", "b"}, resp: Response{Text: "ab"}}}
	o := &out{}
	Service{Providers: resolver{"p1": st}}.Run(context.Background(), cfg(), dec("m1"), o)
	if strings.Join(o.chunks, ",") != "a,b" || len(o.finals) != 0 {
		t.Fatalf("chunks=%v finals=%v", o.chunks, o.finals)
	}
	o2 := &out{}
	Service{Providers: resolver{"p1": &fakeAdapter{resp: Response{Text: "full"}}}}.Run(context.Background(), cfg(), dec("m1"), o2)
	if len(o2.chunks) != 0 || len(o2.finals) != 1 || o2.finals[0] != "full" {
		t.Fatalf("chunks=%v finals=%v", o2.chunks, o2.finals)
	}
}

func TestEXEC3_CancelStopsStreamWithProviderCancel(t *testing.T) {
	cs := &cancelStream{streamAdapter{fakeAdapter: fakeAdapter{chunks: []string{"a", "b"}}, block: make(chan struct{})}}
	ctx, cancel := context.WithCancel(context.Background())
	o := &out{}
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	rec := Service{Providers: resolver{"p1": cs}}.Run(ctx, cfg(), dec("m1"), o)
	if rec.Status != StatusInterrupted || cs.cancels != 1 {
		t.Fatalf("status=%s cancels=%d", rec.Status, cs.cancels)
	}
	for _, n := range o.notices {
		if strings.Contains(n, "could not be cancelled") {
			t.Fatalf("unexpected notice %q", n)
		}
	}
}

func TestEXEC3_NonCancellableAdapterShowsNotice(t *testing.T) {
	st := &streamAdapter{fakeAdapter: fakeAdapter{chunks: []string{"a", "b"}}, block: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	o := &out{}
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	rec := Service{Providers: resolver{"p1": st}}.Run(ctx, cfg(), dec("m1"), o)
	close(st.block) // adapter resumes after cancel; its chunks must not display
	time.Sleep(20 * time.Millisecond)
	o.mu.Lock()
	defer o.mu.Unlock()
	if rec.Status != StatusInterrupted || len(o.notices) != 1 || !strings.Contains(o.notices[0], "could not be cancelled") {
		t.Fatalf("status=%s notices=%v", rec.Status, o.notices)
	}
	if strings.Join(o.chunks, ",") != "a" {
		t.Fatalf("chunks after cancel displayed: %v", o.chunks)
	}
}

func TestEXEC4_ErrorShownNoFallbackNoRetry(t *testing.T) {
	for _, k := range []ErrorKind{KindAuth, KindModelUnavailable, KindProviderFailure, KindInterrupted} {
		a1, a2 := &fakeAdapter{err: &Error{Kind: k, Err: errors.New("boom")}}, &fakeAdapter{}
		o := &out{}
		Service{Providers: resolver{"p1": a1, "p2": a2}}.Run(context.Background(), cfg(), dec("m1"), o)
		if len(a1.calls) != 1 || len(a2.calls) != 0 {
			t.Fatalf("%s: p1=%d p2=%d calls", k, len(a1.calls), len(a2.calls))
		}
		if len(o.errs) == 0 || !strings.Contains(o.errs[0], string(k)) {
			t.Fatalf("%s: errs=%v", k, o.errs)
		}
	}
	// A plain error is normalized to provider_failure.
	o := &out{}
	Service{Providers: resolver{"p1": &fakeAdapter{err: errors.New("x")}}}.Run(context.Background(), cfg(), dec("m1"), o)
	if !strings.Contains(o.errs[0], "provider_failure") {
		t.Fatalf("errs=%v", o.errs)
	}
}

func TestEXEC5_UsageShownOrUnavailable(t *testing.T) {
	o := &out{}
	Service{Providers: resolver{"p1": &fakeAdapter{resp: Response{Usage: Usage{Input: ip(3), Output: ip(4), Total: ip(7)}}}}}.
		Run(context.Background(), cfg(), dec("m1"), o)
	want := "model: m1 | input tokens: 3 | output tokens: 4 | total tokens: 7"
	if o.notices[0] != want {
		t.Fatalf("got %q", o.notices[0])
	}
	o = &out{}
	Service{Providers: resolver{"p1": &fakeAdapter{}}}.Run(context.Background(), cfg(), dec("m1"), o)
	if strings.Count(o.notices[0], "unavailable") != 3 {
		t.Fatalf("got %q", o.notices[0])
	}
}

func TestEXEC6_NoMonetaryCost(t *testing.T) {
	o := &out{}
	Service{Providers: resolver{"p1": &fakeAdapter{resp: Response{Usage: Usage{Input: ip(1), Output: ip(1), Total: ip(2)}}}}}.
		Run(context.Background(), cfg(), dec("m1"), o)
	l := strings.ToLower(strings.Join(o.notices, " "))
	for _, w := range []string{"cost", "$", "usd", "price"} {
		if strings.Contains(l, w) {
			t.Fatalf("output mentions %q: %s", w, l)
		}
	}
}

func TestEXEC7_SendsOnlyStrippedPrompt(t *testing.T) {
	a := &fakeAdapter{}
	d := routing.Route(cfgWithStrategy(), "/code fix this")
	Service{Providers: resolver{"p1": a}}.Run(context.Background(), cfg(), d, &out{})
	if len(a.calls) != 1 || a.calls[0] != "m1|fix this" {
		t.Fatalf("calls=%v", a.calls)
	}
}

func cfgWithStrategy() configuration.Config {
	c := cfg()
	c.Strategies = &configuration.StrategiesFile{Commands: map[string]configuration.CommandAssignment{
		"/code": {Enabled: true, Models: []string{"m1"}},
	}}
	return c
}

func TestEXEC8_OneRecordPerOutcome(t *testing.T) {
	cases := map[string]struct {
		a    Adapter
		want Status
		ctx  func() context.Context
	}{
		"success":     {&fakeAdapter{}, StatusSuccess, context.Background},
		"error":       {&fakeAdapter{err: errors.New("x")}, StatusError, context.Background},
		"interrupted": {&fakeAdapter{}, StatusInterrupted, func() context.Context { c, f := context.WithCancel(context.Background()); f(); return c }},
	}
	for name, c := range cases {
		h := &history{}
		Service{Providers: resolver{"p1": c.a}, History: h}.Run(c.ctx(), cfg(), dec("m1"), &out{})
		if len(h.recs) != 1 || h.recs[0].Status != c.want || h.recs[0].Model != "m1" {
			t.Fatalf("%s: recs=%+v", name, h.recs)
		}
	}
	// Unknown model also yields a record.
	h := &history{}
	Service{Providers: resolver{}, History: h}.Run(context.Background(), cfg(), dec("nope"), &out{})
	if len(h.recs) != 1 || h.recs[0].Status != StatusError {
		t.Fatalf("recs=%+v", h.recs)
	}
}

func TestEXEC9_ErrorsDoNotLeakAPIKeys(t *testing.T) {
	o := &out{}
	a := &fakeAdapter{err: &Error{Kind: KindAuth, Err: errors.New("bad key " + secretKey)}}
	Service{Providers: resolver{"p1": a}}.Run(context.Background(), cfg(), dec("m1"), o)
	if strings.Contains(strings.Join(o.errs, " "), secretKey) || !strings.Contains(o.errs[0], "[redacted]") {
		t.Fatalf("errs=%v", o.errs)
	}
}

func TestEXEC1_NonRoutedDecisionIsNotExecutedNorRecorded(t *testing.T) {
	a, h, o := &fakeAdapter{}, &history{}, &out{}
	for _, k := range []routing.Kind{routing.Refused, routing.ToolCommand} {
		Service{Providers: resolver{"p1": a}, History: h}.Run(context.Background(), cfg(), routing.Decision{Kind: k}, o)
	}
	if len(a.calls) != 0 || len(h.recs) != 0 || len(o.errs) != 2 {
		t.Fatalf("calls=%v recs=%v errs=%v", a.calls, h.recs, o.errs)
	}
}

func TestEXEC5_FinishedRequestIsNotReportedInterrupted(t *testing.T) {
	// Result and cancel are both ready: the real result must win.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan result, 1)
	done <- result{resp: Response{Text: "ok"}}
	stopped := false
	resp, err, abandoned := wait(ctx, done, func() { stopped = true })
	if err != nil || abandoned || resp.Text != "ok" || !stopped {
		t.Fatalf("resp=%v err=%v abandoned=%v stopped=%v", resp, err, abandoned, stopped)
	}
}

func TestEXEC3_ProviderReportedInterruptNeedsNoCancelNotice(t *testing.T) {
	a := &fakeAdapter{err: &Error{Kind: KindInterrupted}}
	o := &out{}
	rec := Service{Providers: resolver{"p1": a}}.Run(context.Background(), cfg(), dec("m1"), o)
	if rec.Status != StatusInterrupted || len(o.notices) != 0 {
		t.Fatalf("status=%s notices=%v", rec.Status, o.notices)
	}
}

func TestEXEC10_SDDPromptSentUnchangedAndTagged(t *testing.T) {
	a := &fakeAdapter{resp: Response{Text: "ok"}}
	h := &history{}
	svc := Service{Providers: resolver{"p1": a, "p2": a}, History: h}
	d := dec("m1")
	d.Prompt = "instruction\n\n=== file.go ===\ncontext"
	rec := svc.RunSDD(context.Background(), cfg(), d, "implement", &out{})
	if a.calls[0] != "m1|"+d.Prompt || rec.SDD != "implement" || h.recs[0].SDD != "implement" {
		t.Fatalf("EXEC-10: %v %+v", a.calls, h.recs)
	}
	svc.Run(context.Background(), cfg(), dec("m1"), &out{})
	if a.calls[1] != "m1|fix this" || h.recs[1].SDD != "" {
		t.Fatalf("EXEC-10: a normal request sends only the stripped prompt: %v", a.calls)
	}
}

func TestEXEC11_ExecDoesNotEnforceSDDLimits(t *testing.T) {
	a := &fakeAdapter{resp: Response{Text: "ok"}}
	svc := Service{Providers: resolver{"p1": a, "p2": a}, History: &history{}}
	d := dec("m1")
	d.Prompt = strings.Repeat("x", 500*1024) // the cap check belongs to SDD (SDD-43)
	svc.RunSDD(context.Background(), cfg(), d, "implement", &out{})
	if len(a.calls) != 1 || len(a.calls[0]) < 500*1024 {
		t.Fatal("EXEC-11: EXEC must send the prompt SDD gives it")
	}
}
