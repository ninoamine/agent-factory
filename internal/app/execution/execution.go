// Package execution runs one routed request on one model and reports the
// outcome. It never retries and never falls back (ARCH-6, EXEC-4).
package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/routing"
	"github.com/ninoamine/agent-factory/internal/domain"
)

// ErrorKind is the normalized provider error (adapter contract, EXEC-4).
type ErrorKind string

const (
	KindAuth             ErrorKind = "auth"
	KindModelUnavailable ErrorKind = "model_unavailable"
	KindProviderFailure  ErrorKind = "provider_failure"
	KindInterrupted      ErrorKind = "interrupted"
)

// Error is an adapter error already normalized to one kind.
type Error struct {
	Kind ErrorKind
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return string(e.Kind)
	}
	return string(e.Kind) + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// Usage holds token counts; nil means unavailable. Counts are never invented (EXEC-5).
type Usage struct {
	Input  *int
	Output *int
	Total  *int
}

// Response is what an adapter returns on success.
type Response struct {
	Text  string
	Usage Usage
}

// Adapter is the required part of a provider adapter (ARCH-2). Implemented in infra.
type Adapter interface {
	Execute(ctx context.Context, modelID, prompt string) (Response, error)
}

// Streamer is the optional streaming part (EXEC-2). onChunk receives text pieces.
type Streamer interface {
	Stream(ctx context.Context, modelID, prompt string, onChunk func(string)) (Response, error)
}

// Canceller is the optional provider-side cancel (EXEC-3).
type Canceller interface {
	Cancel() error
}

// Resolver finds the adapter of a provider.
type Resolver interface {
	Executor(provider string) (Adapter, bool)
}

// Output is where the CLI displays results. Implemented by the caller.
type Output interface {
	Chunk(text string)  // incremental text (EXEC-2)
	Final(text string)  // full text when not streaming
	Notice(text string) // usage line, cancel notice
	Error(text string)  // normalized error
}

// Status is the outcome of a request.
type Status string

const (
	StatusSuccess     Status = "success"
	StatusError       Status = "error"
	StatusInterrupted Status = "interrupted"
)

// Record is passed to history after every request (EXEC-8).
type Record struct {
	Time      time.Time
	Command   domain.RoutingCommand
	Model     domain.ModelRef
	Prompt    string
	Status    Status
	ErrorKind ErrorKind // set unless Status == StatusSuccess
	Usage     Usage
	SDD       string // /sdd subcommand for SDD runs, empty otherwise (HIST-11)
}

// Recorder is implemented by the history service (HIST-1).
type Recorder interface {
	Record(ctx context.Context, r Record) error
}

// Service executes routed requests.
type Service struct {
	Providers Resolver
	History   Recorder
	Now       func() time.Time // defaults to time.Now
}

// Run executes one routed decision (ARCH-10). It calls only the model chosen
// by routing (EXEC-1) and sends only the stripped prompt (EXEC-7).
func (s Service) Run(ctx context.Context, cfg configuration.Config, d routing.Decision, out Output) Record {
	return s.RunSDD(ctx, cfg, d, "", out)
}

// RunSDD is Run for an SDD subcommand: d.Prompt carries the file context built
// by SDD and is sent unchanged (EXEC-10). EXEC adds no history or context of
// its own and does not enforce SDD limits (EXEC-11). sdd tags the history record.
func (s Service) RunSDD(ctx context.Context, cfg configuration.Config, d routing.Decision, sdd string, out Output) Record {
	if d.Kind != routing.Routed {
		// Refused and tool-command decisions are never executed or recorded (ROUTE-4, ROUTE-7).
		out.Error("nothing to execute: the input was not routed to a model")
		return Record{}
	}
	rec := Record{Time: s.now(), Command: d.Command, Model: d.Model, Prompt: d.Prompt, SDD: sdd}

	adapter, modelID, err := s.resolve(cfg, d.Model)
	if err != nil {
		rec.Status, rec.ErrorKind = StatusError, KindModelUnavailable
		out.Error(format(KindModelUnavailable, redact(err.Error(), cfg)))
		s.record(ctx, cfg, rec, out)
		return rec
	}

	resp, execErr, abandoned := call(ctx, adapter, modelID, d.Prompt, out)
	switch {
	case abandoned || isKind(execErr, KindInterrupted) || (execErr != nil && ctx.Err() != nil):
		rec.Status, rec.ErrorKind = StatusInterrupted, KindInterrupted
		// Only a user cancel (context done) needs a provider cancel or the notice (EXEC-3).
		if ctx.Err() != nil {
			if c, ok := adapter.(Canceller); ok {
				if cerr := c.Cancel(); cerr != nil {
					out.Notice("the provider could not be cancelled: " + redact(cerr.Error(), cfg))
				}
			} else {
				out.Notice("the provider could not be cancelled; output is no longer displayed")
			}
		}
		out.Error(format(KindInterrupted, "request stopped"))
	case execErr != nil:
		kind := kindOf(execErr)
		rec.Status, rec.ErrorKind = StatusError, kind
		out.Error(format(kind, redact(execErr.Error(), cfg)))
	default:
		rec.Status, rec.Usage = StatusSuccess, resp.Usage
		if _, streamed := adapter.(Streamer); !streamed {
			out.Final(resp.Text)
		}
		out.Notice(FormatUsage(string(d.Model), resp.Usage)) // EXEC-5; no cost (EXEC-6)
	}
	s.record(ctx, cfg, rec, out)
	return rec
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Service) resolve(cfg configuration.Config, ref domain.ModelRef) (Adapter, string, error) {
	if cfg.Models == nil {
		return nil, "", errors.New("models.json is missing or invalid")
	}
	for _, m := range cfg.Models.Models {
		if m.ID != string(ref) {
			continue
		}
		a, ok := s.Providers.Executor(m.Provider)
		if !ok || a == nil {
			return nil, "", fmt.Errorf("no adapter for provider %q (model %q)", m.Provider, m.ID)
		}
		return a, m.ID, nil
	}
	return nil, "", fmt.Errorf("model %q is not in models.json", ref)
}

// call runs the adapter in a goroutine so Ctrl+C returns promptly even when
// the adapter ignores the context. Chunks arriving after cancel are dropped.
// abandoned is true when call returned without the adapter finishing.
func call(ctx context.Context, a Adapter, modelID, prompt string, out Output) (Response, error, bool) {
	var mu sync.Mutex
	stopped := false
	onChunk := func(t string) {
		mu.Lock()
		defer mu.Unlock()
		if !stopped {
			out.Chunk(t)
		}
	}
	done := make(chan result, 1)
	go func() {
		var r result
		if st, ok := a.(Streamer); ok {
			r.resp, r.err = st.Stream(ctx, modelID, prompt, onChunk)
		} else {
			r.resp, r.err = a.Execute(ctx, modelID, prompt)
		}
		done <- r
	}()
	return wait(ctx, done, func() { mu.Lock(); stopped = true; mu.Unlock() })
}

type result struct {
	resp Response
	err  error
}

// wait returns the adapter result, or abandons it when ctx is done. If both
// are ready, the real result wins. stop is called first so no more chunks
// are displayed.
func wait(ctx context.Context, done <-chan result, stop func()) (Response, error, bool) {
	select {
	case r := <-done:
		stop()
		return r.resp, r.err, false
	case <-ctx.Done():
		stop()
		select {
		case r := <-done: // finished at the same moment: keep the real result
			return r.resp, r.err, false
		default:
		}
		return Response{}, ctx.Err(), true
	}
}

func (s Service) record(ctx context.Context, cfg configuration.Config, rec Record, out Output) {
	if s.History == nil {
		return
	}
	// The request may have been cancelled; history must still be written.
	if err := s.History.Record(context.WithoutCancel(ctx), rec); err != nil {
		out.Error("could not save history: " + redact(err.Error(), cfg))
	}
}

func kindOf(err error) ErrorKind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindProviderFailure
}

func isKind(err error, k ErrorKind) bool {
	var e *Error
	return errors.As(err, &e) && e.Kind == k
}

func format(k ErrorKind, detail string) string {
	return fmt.Sprintf("error (%s): %s", k, detail)
}

// FormatUsage renders model and token counts; unknown counts say "unavailable" (EXEC-5).
func FormatUsage(model string, u Usage) string {
	n := func(p *int) string {
		if p == nil {
			return "unavailable"
		}
		return fmt.Sprint(*p)
	}
	return fmt.Sprintf("model: %s | input tokens: %s | output tokens: %s | total tokens: %s",
		model, n(u.Input), n(u.Output), n(u.Total))
}

// redact removes API keys from text (EXEC-9, SEC-1).
func redact(s string, cfg configuration.Config) string {
	if cfg.Models == nil {
		return s
	}
	for _, p := range cfg.Models.Providers {
		if strings.TrimSpace(p.APIKey) != "" {
			s = strings.ReplaceAll(s, p.APIKey, "[redacted]")
		}
	}
	return s
}
