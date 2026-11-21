package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
	"github.com/ninoamine/agent-factory/internal/app/routing"
	"github.com/ninoamine/agent-factory/internal/app/strategies"
)

const secret = "sk-test-SECRET"

// ---- fakes ----

// fakeAdapter streams "Hel", waits for the gate, then streams "lo" (usage 3/2/5).
type fakeAdapter struct {
	gate     chan struct{}
	first    chan struct{}
	calls    atomic.Int32
	err      error
	onceChnk sync.Once
}

func newAdapter() *fakeAdapter {
	return &fakeAdapter{gate: make(chan struct{}), first: make(chan struct{})}
}

func (a *fakeAdapter) Execute(ctx context.Context, model, prompt string) (execution.Response, error) {
	return execution.Response{}, errors.New("not used")
}

func (a *fakeAdapter) Stream(ctx context.Context, model, prompt string, onChunk func(string)) (execution.Response, error) {
	a.calls.Add(1)
	if a.err != nil {
		return execution.Response{}, a.err
	}
	onChunk("Hel")
	a.onceChnk.Do(func() { close(a.first) })
	select {
	case <-a.gate:
	case <-ctx.Done():
		return execution.Response{}, ctx.Err()
	}
	onChunk("lo")
	i, o, t := 3, 2, 5
	return execution.Response{Text: "Hello", Usage: execution.Usage{Input: &i, Output: &o, Total: &t}}, nil
}

func (a *fakeAdapter) Cancel() error { return nil }

// plainAdapter has no Cancel method (EXEC-3 notice).
type plainAdapter struct{ a *fakeAdapter }

func (p plainAdapter) Execute(ctx context.Context, m, pr string) (execution.Response, error) {
	return p.a.Execute(ctx, m, pr)
}

func (p plainAdapter) Stream(ctx context.Context, m, pr string, f func(string)) (execution.Response, error) {
	return p.a.Stream(ctx, m, pr, f)
}

type resolver struct{ a execution.Adapter }

func (r resolver) Executor(string) (execution.Adapter, bool) { return r.a, true }

type fakeBackend struct {
	cfg      configuration.Config
	startup  Startup
	loadGate chan struct{}
	loadErr  error
	adapter  execution.Adapter

	suggestText string

	configCalls  atomic.Int32
	refreshCalls atomic.Int32
	historyCalls atomic.Int32
	cleared      atomic.Bool
	historyGate  chan struct{} // when set, History blocks until it is closed
}

func fixtureConfig() configuration.Config {
	return configuration.Config{
		Dir: "/tmp/af",
		Models: &configuration.ModelsFile{
			Providers: map[string]configuration.ProviderEntry{"p": {APIKey: secret}},
			Models:    []configuration.ModelEntry{{ID: "m1", Provider: "p"}, {ID: "m2", Provider: "p"}},
		},
		Strategies: &configuration.StrategiesFile{
			Strategy: "cost",
			Commands: map[string]configuration.CommandAssignment{
				"/fast":     {Enabled: true, Models: []string{"m1"}},
				"/code":     {Enabled: true, Models: []string{"m2"}},
				"/reason":   {Enabled: true, Models: []string{"m2"}},
				"/creative": {Enabled: false},
			},
		},
	}
}

func newBackend(a execution.Adapter) *fakeBackend {
	cfg := fixtureConfig()
	return &fakeBackend{cfg: cfg, adapter: a, startup: Startup{Summary: cfg.Summary()}}
}

func (b *fakeBackend) Load(ctx context.Context) (Startup, error) {
	if b.loadErr != nil {
		return Startup{}, b.loadErr
	}
	if b.loadGate != nil {
		select {
		case <-b.loadGate:
		case <-ctx.Done():
			return Startup{}, ctx.Err()
		}
	}
	return b.startup, nil
}

func (b *fakeBackend) Config() configuration.Config {
	b.configCalls.Add(1)
	return b.cfg
}

func (b *fakeBackend) Route(input string) routing.Decision { return routing.Route(b.cfg, input) }

func (b *fakeBackend) Execute(ctx context.Context, d routing.Decision, out execution.Output) {
	execution.Service{Providers: resolver{b.adapter}}.Run(ctx, b.cfg, d, out)
}

func (b *fakeBackend) Suggest(ctx context.Context, args string) (string, error) {
	return b.suggestText, nil
}

func (b *fakeBackend) Refresh(ctx context.Context) (configuration.RefreshOutcome, error) {
	b.refreshCalls.Add(1)
	return configuration.RefreshOutcome{Applied: true, Summary: b.cfg.Summary()}, nil
}

func (b *fakeBackend) History(ctx context.Context, args string) (HistoryReply, error) {
	b.historyCalls.Add(1)
	if b.historyGate != nil {
		<-b.historyGate
	}
	if strings.HasPrefix(args, "clear") {
		return HistoryReply{Confirm: &Confirmation{
			Prompt: "Delete all history entries? Type y or yes to confirm.",
			Run: func(ctx context.Context) ([]string, error) {
				b.cleared.Store(true)
				return []string{"history cleared"}, nil
			},
		}}, nil
	}
	return HistoryReply{Lines: []string{"entry-1"}}, nil
}

func (b *fakeBackend) HistoryMutates(args string) bool {
	return strings.HasPrefix(args, "delete") || strings.HasPrefix(args, "clear")
}

// ---- harness: drives the model without a terminal ----

type harness struct {
	t    *testing.T
	m    *Model
	msgs chan tea.Msg
}

func newHarness(t *testing.T, b *fakeBackend) *harness {
	t.Helper()
	h := &harness{t: t, msgs: make(chan tea.Msg, 1024)}
	h.m = New(b)
	h.m.SetSender(func(msg tea.Msg) { h.msgs <- msg })
	h.run(h.m.Init())
	h.feed(tea.WindowSizeMsg{Width: 80, Height: 24})
	return h
}

func (h *harness) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() {
		if msg := cmd(); msg != nil {
			h.msgs <- msg
		}
	}()
}

func (h *harness) feed(msg tea.Msg) {
	_, cmd := h.m.Update(msg)
	h.run(cmd)
}

func (h *harness) send(text string) {
	h.m.input.SetValue(text)
	h.feed(tea.KeyMsg{Type: tea.KeyEnter})
}

func (h *harness) key(k tea.KeyType) { h.feed(tea.KeyMsg{Type: k}) }

// waitFor processes messages until cond holds (2 s limit).
func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.After(2 * time.Second)
	for !cond() {
		select {
		case msg := <-h.msgs:
			h.feed(msg)
		case <-time.After(5 * time.Millisecond):
		case <-deadline:
			h.t.Fatalf("timed out waiting for %s; screen:\n%s", what, h.m.View())
		}
	}
}

func (h *harness) waitScreen(sub string) {
	h.t.Helper()
	h.waitFor("screen to contain "+sub, func() bool { return strings.Contains(h.m.View(), sub) })
}

// settle processes pending messages for a short moment.
func (h *harness) settle() {
	for {
		select {
		case msg := <-h.msgs:
			h.feed(msg)
		case <-time.After(30 * time.Millisecond):
			return
		}
	}
}

func (h *harness) all() string {
	var b strings.Builder
	for _, l := range h.m.out.lines {
		b.WriteString(l.text + "\n")
	}
	return b.String()
}

func ready(t *testing.T, b *fakeBackend) *harness {
	t.Helper()
	h := newHarness(t, b)
	h.waitFor("startup to finish", func() bool { return h.m.loaded })
	return h
}

func lineIndex(s, sub string) int { return strings.Index(s, sub) }

// ---- tests ----

func TestTUI1_LayoutOutputAboveOneLineInput(t *testing.T) { // AC1
	t.Run("TUI-1", func(t *testing.T) {
		h := ready(t, newBackend(newAdapter()))
		rows := strings.Split(h.m.View(), "\n")
		if len(rows) != 24 {
			t.Fatalf("want 24 rows, got %d", len(rows))
		}
		if !strings.HasPrefix(rows[23], "> ") {
			t.Fatalf("last row should be the input box, got %q", rows[23])
		}
		if !strings.Contains(strings.Join(rows[:23], "\n"), "Providers (1)") {
			t.Fatal("startup summary should be in the output area above the input")
		}
	})
}

func TestTUI3_TUI4_StreamedOutputLabelAndUsage(t *testing.T) { // AC2
	t.Run("TUI-3 TUI-4", func(t *testing.T) {
		a := newAdapter()
		h := ready(t, newBackend(a))
		h.send("/code hi")
		h.waitScreen("Hel")
		if scr := h.m.View(); strings.Contains(scr, "Hello") || !strings.Contains(scr, "m2:") {
			t.Fatalf("TUI-3/4: want label and partial chunk before the rest arrives:\n%s", scr)
		}
		close(a.gate)
		h.waitScreen("Hello")
		h.waitScreen("model: m2 | input tokens: 3 | output tokens: 2 | total tokens: 5")
		all := h.all()
		if !(lineIndex(all, "m2:") < lineIndex(all, "Hello") && lineIndex(all, "Hello") < lineIndex(all, "input tokens: 3")) {
			t.Fatalf("TUI-3: order must be label, text, usage:\n%s", all)
		}
	})
}

func TestTUI5_TUI16_CtrlCCancelsAndReturnsToIdle(t *testing.T) { // AC3
	t.Run("TUI-5 TUI-16", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			adapter  func(*fakeAdapter) execution.Adapter
			wantNote bool
		}{
			{"cancellable", func(a *fakeAdapter) execution.Adapter { return a }, false},
			{"non-cancellable", func(a *fakeAdapter) execution.Adapter { return plainAdapter{a} }, true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				a := newAdapter()
				h := ready(t, newBackend(tc.adapter(a)))
				h.send("/code hi")
				h.waitScreen("Hel")
				h.key(tea.KeyCtrlC)
				h.waitScreen("error: interrupted")
				h.waitFor("idle input", func() bool { return !h.m.running })
				if h.m.Quitting() {
					t.Fatal("TUI-5: Ctrl+C while a request runs must not exit")
				}
				if got := strings.Contains(h.all(), "could not be cancelled"); got != tc.wantNote {
					t.Fatalf("TUI-16: notice shown = %v, want %v\n%s", got, tc.wantNote, h.all())
				}
			})
		}
	})
}

func TestTUI9_TUI7_ModelsListsFieldsWithoutKeys(t *testing.T) { // AC4
	t.Run("TUI-9 TUI-7", func(t *testing.T) {
		h := ready(t, newBackend(newAdapter()))
		h.send("/models")
		h.waitScreen("m1 | provider: p | commands: /fast")
		h.waitScreen("m2 | provider: p | commands: /code, /reason")
		if strings.Contains(h.all(), secret) || strings.Contains(h.m.View(), secret) {
			t.Fatal("TUI-7: the key must never be displayed")
		}
	})
}

func TestTUI7_NoKeyInSummaryErrorOrHelp(t *testing.T) { // AC5
	t.Run("TUI-7", func(t *testing.T) {
		a := newAdapter()
		a.err = &execution.Error{Kind: execution.KindAuth, Err: errors.New("rejected key " + secret)}
		h := ready(t, newBackend(a))
		h.send("/code hi")
		h.waitScreen("auth")
		h.send("/help")
		h.waitScreen("/refresh")
		if strings.Contains(h.all(), secret) {
			t.Fatalf("TUI-7: key leaked:\n%s", h.all())
		}
	})
}

func TestTUI6_WarningAndErrorPrefixesWithoutColor(t *testing.T) { // AC6
	t.Run("TUI-6", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		a := newAdapter()
		a.err = &execution.Error{Kind: execution.KindProviderFailure, Err: errors.New("boom")}
		b := newBackend(a)
		b.startup.Warnings = []string{"/tmp/af/models.json is missing"}
		h := ready(t, b)
		h.waitScreen("warning: /tmp/af/models.json is missing")
		h.send("/code hi")
		h.waitScreen("error: provider_failure: boom")
		if strings.Contains(h.all(), "provider_failure: provider_failure") {
			t.Fatal("the error kind must not repeat")
		}
	})
}

func TestTUI2_DispatchByFirstWord(t *testing.T) { // AC7
	t.Run("TUI-2", func(t *testing.T) {
		a := newAdapter()
		b := newBackend(a)
		h := ready(t, b)
		h.send("hello")
		h.waitScreen("error: input must start with a routing command")
		before := b.configCalls.Load()
		h.send("/models extra")
		h.waitScreen("m1 | provider")
		if b.configCalls.Load() == before {
			t.Fatal("TUI-2: /models should reach its handler")
		}
		if a.calls.Load() != 0 {
			t.Fatal("TUI-2: neither line may reach the provider")
		}
	})
}

func TestTUI8_HelpListsAllCommands(t *testing.T) { // AC8
	t.Run("TUI-8", func(t *testing.T) {
		h := ready(t, newBackend(newAdapter()))
		h.send("/help")
		h.waitScreen("/creative")
		for _, c := range []string{"/refresh", "/suggest", "/models", "/history", "/help", "/fast", "/code", "/reason", "/creative"} {
			if !strings.Contains(h.all(), c) {
				t.Errorf("TUI-8: /help misses %s", c)
			}
		}
	})
}

func TestTUI10_TUI11_LoadingLineThenOrderedResults(t *testing.T) { // AC9
	t.Run("TUI-10 TUI-11", func(t *testing.T) {
		b := newBackend(newAdapter())
		b.loadGate = make(chan struct{})
		b.startup = Startup{
			Warnings:         []string{"models.json is missing"},
			Problems:         []configuration.Problem{{File: "strategies.json", Path: "$.strategy", Reason: "bad"}},
			MetadataWarnings: []string{"metadata fetch failed for m1"},
			Summary:          "SUMMARY-LINE",
		}
		h := newHarness(t, b)
		if !strings.Contains(h.m.View(), "loading") {
			t.Fatalf("TUI-10: want a loading line:\n%s", h.m.View())
		}
		close(b.loadGate)
		h.waitScreen("SUMMARY-LINE")
		all := h.all()
		order := []string{"warning: models.json is missing", "error: strategies.json", "warning: metadata fetch failed", "SUMMARY-LINE"}
		last := -1
		for _, s := range order {
			i := lineIndex(all, s)
			if i <= last {
				t.Fatalf("TUI-11: %q out of order in:\n%s", s, all)
			}
			last = i
		}
	})
}

func TestTUI12_LineBeforeLoadingFinishesIsRefused(t *testing.T) { // AC10
	t.Run("TUI-12", func(t *testing.T) {
		b := newBackend(newAdapter())
		b.loadGate = make(chan struct{})
		h := newHarness(t, b)
		h.send("/models")
		if !strings.Contains(h.m.View(), "startup is in progress") {
			t.Fatalf("want a refusal:\n%s", h.m.View())
		}
		if b.configCalls.Load() != 0 {
			t.Fatal("TUI-12: nothing may be dispatched")
		}
	})
}

func TestTUI13_TUI14_WhileRequestRuns(t *testing.T) { // AC11
	t.Run("TUI-13 TUI-14", func(t *testing.T) {
		a := newAdapter()
		h := ready(t, newBackend(a))
		h.send("/code hi")
		h.waitScreen("Hel")
		h.send("/models")
		h.waitScreen("m1 | provider")
		h.send("/help")
		h.waitScreen("Tool commands:")
		h.send("/history")
		h.waitScreen("entry-1")
		h.send("/suggest cost")
		h.settle()
		if !h.m.running {
			t.Fatal("TUI-13: the stream must keep going")
		}
		h.send("/code again")
		h.waitScreen("error: a request is already running")
		if a.calls.Load() != 1 {
			t.Fatalf("TUI-14: provider called %d times, want 1", a.calls.Load())
		}
		close(a.gate)
		h.waitScreen("total tokens: 5")
	})
}

func TestTUI15_MutatingCommandsRefusedWhileRunning(t *testing.T) { // AC12
	t.Run("TUI-15", func(t *testing.T) {
		a := newAdapter()
		b := newBackend(a)
		h := ready(t, b)
		h.send("/code hi")
		h.waitScreen("Hel")
		h.send("/history clear")
		h.send("/history delete abc")
		h.send("/refresh")
		h.settle()
		if b.historyCalls.Load() != 0 || b.refreshCalls.Load() != 0 {
			t.Fatal("TUI-15: neither command may reach the application")
		}
		if !strings.Contains(h.all(), "refused") {
			t.Fatalf("TUI-15: want a refusal message:\n%s", h.all())
		}
		close(a.gate)
		h.waitScreen("total tokens: 5")
	})
}

func TestTUI15_ConfirmAnsweredAfterRequestStartedIsRefused(t *testing.T) {
	t.Run("TUI-15 clear confirmed while a request runs", func(t *testing.T) {
		a := newAdapter()
		b := newBackend(a)
		b.historyGate = make(chan struct{})
		h := ready(t, b)
		h.send("/history clear") // the prompt is still being fetched
		h.send("/code hi")       // a request starts before the prompt arrives
		h.waitScreen("Hel")
		close(b.historyGate)
		h.waitScreen("Type y or yes")
		h.send("y")
		h.settle()
		if b.cleared.Load() {
			t.Fatal("TUI-15: history must not be cleared while a request runs")
		}
		if !strings.Contains(h.all(), "refused") {
			t.Fatalf("want a refusal:\n%s", h.all())
		}
		close(a.gate)
		h.waitScreen("total tokens: 5")
	})
}

func TestTUI17_TUI18_ExitKeys(t *testing.T) { // AC13
	t.Run("TUI-17 TUI-18", func(t *testing.T) {
		t.Run("TUI-17 Ctrl+C idle", func(t *testing.T) {
			h := ready(t, newBackend(newAdapter()))
			h.key(tea.KeyCtrlC)
			if !h.m.Quitting() {
				t.Fatal("want exit")
			}
		})
		t.Run("TUI-17 Ctrl+C during startup", func(t *testing.T) {
			b := newBackend(newAdapter())
			b.loadGate = make(chan struct{})
			h := newHarness(t, b)
			h.key(tea.KeyCtrlC)
			if !h.m.Quitting() {
				t.Fatal("want exit")
			}
		})
		t.Run("TUI-18 Ctrl+D empty", func(t *testing.T) {
			h := ready(t, newBackend(newAdapter()))
			h.key(tea.KeyCtrlD)
			if !h.m.Quitting() {
				t.Fatal("want exit")
			}
		})
		t.Run("TUI-18 Ctrl+D with text", func(t *testing.T) {
			h := ready(t, newBackend(newAdapter()))
			h.m.input.SetValue("abc")
			h.key(tea.KeyCtrlD)
			if h.m.Quitting() {
				t.Fatal("must do nothing")
			}
		})
	})
}

func TestTUI19_ConfirmationPrompt(t *testing.T) { // AC14
	t.Run("TUI-19", func(t *testing.T) {
		t.Run("n cancels", func(t *testing.T) {
			b := newBackend(newAdapter())
			h := ready(t, b)
			h.send("/history clear")
			h.waitFor("prompt", func() bool { return h.m.confirm != nil })
			h.send("n")
			h.settle()
			if b.cleared.Load() {
				t.Fatal("history must stay")
			}
		})
		t.Run("yes confirms", func(t *testing.T) {
			b := newBackend(newAdapter())
			h := ready(t, b)
			h.send("/history clear")
			h.waitFor("prompt", func() bool { return h.m.confirm != nil })
			h.send("yes")
			h.waitScreen("history cleared")
			if !b.cleared.Load() {
				t.Fatal("history should be cleared")
			}
		})
		t.Run("a command is an answer, not dispatched", func(t *testing.T) {
			b := newBackend(newAdapter())
			h := ready(t, b)
			h.send("/history clear")
			h.waitFor("prompt", func() bool { return h.m.confirm != nil })
			before := b.configCalls.Load()
			h.send("/models")
			h.settle()
			if b.cleared.Load() || b.configCalls.Load() != before || h.m.confirm != nil {
				t.Fatal("TUI-19: /models must cancel the prompt and not run")
			}
		})
		t.Run("Ctrl+C cancels the prompt only", func(t *testing.T) {
			h := ready(t, newBackend(newAdapter()))
			h.send("/history clear")
			h.waitFor("prompt", func() bool { return h.m.confirm != nil })
			h.key(tea.KeyCtrlC)
			if h.m.confirm != nil || h.m.Quitting() {
				t.Fatal("TUI-19: the prompt is cancelled and the app keeps running")
			}
		})
	})
}

func TestTUI20_TUI21_Scrolling(t *testing.T) { // AC15
	t.Run("TUI-20 TUI-21", func(t *testing.T) {
		b := newBackend(newAdapter())
		var sb strings.Builder
		for i := 1; i <= 100; i++ {
			fmt.Fprintf(&sb, "row-%03d\n", i)
		}
		b.suggestText = sb.String()
		h := ready(t, b)
		h.send("/suggest x")
		h.waitScreen("row-100")
		h.key(tea.KeyPgUp)
		if strings.Contains(h.m.View(), "row-100") {
			t.Fatal("TUI-21: PgUp should move the view up")
		}
		before := h.m.View()
		h.feed(outMsg{kind: kindInfo, text: "NEWEST"})
		if h.m.View() != before {
			t.Fatal("TUI-20: new output must not move a scrolled-up view")
		}
		for range 10 {
			h.key(tea.KeyPgDown)
		}
		h.feed(outMsg{kind: kindInfo, text: "NEWEST-2"})
		if !strings.Contains(h.m.View(), "NEWEST-2") {
			t.Fatal("TUI-20: at the bottom the newest line must show")
		}
	})
}

func TestTUI23_LongModelIDKeepsJSONValid(t *testing.T) {
	t.Run("TUI-23 long id breaks between tokens", func(t *testing.T) {
		long := "provider/" + strings.Repeat("m", 66)
		sug, err := strategies.Suggest("capability", []metadata.Entry{
			{ID: long, Capabilities: []string{"code", "reasoning", "creative"}, Speed: "fast"},
		})
		if err != nil {
			t.Fatal(err)
		}
		text, err := sug.Render("/tmp/af")
		if err != nil {
			t.Fatal(err)
		}
		b := newBackend(newAdapter())
		b.suggestText = text
		h := ready(t, b)
		h.feed(tea.WindowSizeMsg{Width: 80, Height: 60})
		h.send("/suggest capability")
		h.waitScreen("Paste into")
		var block []string
		in := false
		for _, r := range strings.Split(h.m.View(), "\n") {
			switch {
			case strings.HasPrefix(r, "```json"):
				in = true
			case strings.HasPrefix(r, "```") && in:
				in = false
			case in:
				block = append(block, r)
			}
		}
		var f configuration.StrategiesFile
		if err := json.Unmarshal([]byte(strings.Join(block, "\n")), &f); err != nil {
			t.Fatalf("copied block is not JSON: %v\n%s", err, strings.Join(block, "\n"))
		}
		if errs := f.Validate(); len(errs) > 0 {
			t.Fatalf("fails strategies.json schema: %v", errs)
		}
	})
}

func TestTUI22_SoftWrapAndReflow(t *testing.T) { // AC16
	t.Run("TUI-22", func(t *testing.T) {
		b := newBackend(newAdapter())
		b.suggestText = strings.Repeat("a", 200)
		h := ready(t, b)
		h.send("/suggest x")
		h.waitScreen(strings.Repeat("a", 40))
		widths := func() []int {
			var w []int
			for _, r := range strings.Split(h.m.View(), "\n") {
				if r != "" && strings.Trim(r, "a") == "" {
					w = append(w, len(r))
				}
			}
			return w
		}
		if got := fmt.Sprint(widths()); got != "[80 80 40]" {
			t.Fatalf("want wrap at 80, got %s", got)
		}
		h.feed(tea.WindowSizeMsg{Width: 120, Height: 24})
		if got := fmt.Sprint(widths()); got != "[120 80]" {
			t.Fatalf("want reflow at 120, got %s", got)
		}
	})
	t.Run("TUI-22 wraps at a space, not inside a word", func(t *testing.T) {
		b := newBackend(newAdapter())
		b.suggestText = strings.TrimSpace(strings.Repeat("words ", 40))
		h := ready(t, b)
		h.send("/suggest x")
		h.waitScreen("words")
		rows := 0
		for _, r := range strings.Split(h.m.View(), "\n") {
			if !strings.Contains(r, "words") || strings.HasPrefix(r, "> ") {
				continue
			}
			rows++
			for _, w := range strings.Fields(r) {
				if w != "words" {
					t.Fatalf("a word was split: %q in %q", w, r)
				}
			}
		}
		if rows < 3 {
			t.Fatalf("want the 239-character line wrapped over several rows, got %d", rows)
		}
	})
}

func TestTUI23_SuggestJSONCopiesAsValidJSON(t *testing.T) { // AC17
	t.Run("TUI-23", func(t *testing.T) {
		sug, err := strategies.Suggest("capability", []metadata.Entry{
			{ID: "m1", Capabilities: []string{"code", "reasoning"}, Speed: "fast"},
			{ID: "m2", Capabilities: []string{"creative"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		text, err := sug.Render("/tmp/af")
		if err != nil {
			t.Fatal(err)
		}
		b := newBackend(newAdapter())
		b.suggestText = text
		h := ready(t, b)
		h.feed(tea.WindowSizeMsg{Width: 80, Height: 60})
		h.send("/suggest capability")
		h.waitScreen("Paste into")

		rows := strings.Split(h.m.View(), "\n")
		var block []string
		in := false
		for _, r := range rows {
			switch {
			case strings.HasPrefix(r, "```json"):
				in = true
			case strings.HasPrefix(r, "```") && in:
				in = false
			case in:
				block = append(block, r)
			}
		}
		var f configuration.StrategiesFile
		if err := json.Unmarshal([]byte(strings.Join(block, "\n")), &f); err != nil {
			t.Fatalf("TUI-23: copied block is not JSON: %v\n%s", err, strings.Join(block, "\n"))
		}
		if errs := f.Validate(); len(errs) > 0 {
			t.Fatalf("TUI-23: fails strategies.json schema: %v", errs)
		}
		// No added prefix, border or indentation: identical to the application text.
		want := strings.Split(text, "\n")
		for _, l := range block {
			found := false
			for _, w := range want {
				if w == l {
					found = true
				}
			}
			if !found {
				t.Fatalf("TUI-23: line %q was altered", l)
			}
		}
	})
}

func TestTUI6_ExecutionErrorFormat(t *testing.T) {
	t.Run("TUI-6", func(t *testing.T) {
		if got := formatError("error (auth): bad"); got != "auth: bad" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestTUI12_FailedLoadKeepsLinesRefused(t *testing.T) {
	t.Run("TUI-12 failed startup", func(t *testing.T) {
		b := newBackend(newAdapter())
		b.loadErr = errors.New("disk on fire")
		h := newHarness(t, b)
		h.waitScreen("error: startup failed: disk on fire")
		h.send("/models")
		h.settle()
		if !strings.Contains(h.all(), "startup failed, nothing can run") || b.configCalls.Load() != 0 {
			t.Fatalf("nothing may be dispatched after a failed load:\n%s", h.all())
		}
		h.key(tea.KeyCtrlC)
		if !h.m.Quitting() {
			t.Fatal("TUI-17: Ctrl+C must still exit")
		}
	})
}

func TestTUI17_WaitBlocksUntilRequestFinishes(t *testing.T) {
	t.Run("TUI-17 Wait lets an interrupted request finish (EXEC-8)", func(t *testing.T) {
		a := newAdapter()
		h := ready(t, newBackend(a))
		h.send("/code hi")
		h.waitScreen("Hel")
		done := make(chan struct{})
		go func() { h.m.Wait(); close(done) }()
		select {
		case <-done:
			t.Fatal("Wait returned while the request was still running")
		case <-time.After(50 * time.Millisecond):
		}
		close(a.gate)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Wait did not return after the request finished")
		}
	})
}
