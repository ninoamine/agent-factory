// Package cli is the terminal screen (Bubble Tea, D-016). It does presentation
// and dispatch only; all logic lives in the application layer (ARCH-3).
package cli

import (
	"context"
	"errors"
	"strings"
	"sync"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/history"
	"github.com/ninoamine/agent-factory/internal/app/routing"
	"github.com/ninoamine/agent-factory/internal/app/sdd"
	"github.com/ninoamine/agent-factory/internal/domain"
)

// Startup is the result of loading configuration and metadata (TUI-11).
type Startup struct {
	Warnings         []string                // missing-file and other config warnings (CFG-2)
	Problems         []configuration.Problem // configuration errors
	MetadataWarnings []string                // META-8
	Summary          string                  // CFG-7
}

// Confirmation is a question the application asks before a destructive action
// (TUI-19). The only one in V1 is /history clear, so the type comes from history.
type Confirmation = history.Confirmation

// HistoryReply is the outcome of a /history command.
type HistoryReply = history.Reply

// SDDReply is the outcome of a /sdd command or an approval answer.
type SDDReply = sdd.Reply

// Backend is what the screen needs from the application layer. It is defined
// here, where it is consumed (CONV-4), and wired in cmd/agent-factory.
type Backend interface {
	Load(ctx context.Context) (Startup, error)
	Config() configuration.Config
	Route(input string) routing.Decision
	// Execute runs a routed decision and blocks until it ends.
	Execute(ctx context.Context, d routing.Decision, out execution.Output)
	Suggest(ctx context.Context, args string) (string, error)
	Refresh(ctx context.Context) (configuration.RefreshOutcome, error)
	History(ctx context.Context, args string) (HistoryReply, error)
	// HistoryMutates reports whether the /history arguments delete data (TUI-15).
	HistoryMutates(args string) bool
	// SDDKind returns the subcommand name when /sdd args run a model, else "" (SDD-40).
	SDDKind(args string) string
	// SDD runs a /sdd command; out receives streamed model output.
	SDD(ctx context.Context, args string, out execution.Output) (SDDReply, error)
	// SDDAnswer answers the approval prompt of a waiting proposal (SDD-49).
	SDDAnswer(ctx context.Context, line string, out execution.Output) (SDDReply, error)
}

// Model is the Bubble Tea model of the screen.
type Model struct {
	backend Backend
	send    func(tea.Msg)

	width, height int
	input         textinput.Model
	out           output

	loaded     bool
	loadFailed bool
	loadCancel context.CancelFunc

	running   bool
	runCancel context.CancelFunc
	inflight  sync.WaitGroup // in-flight request goroutines, see Wait

	confirm *Confirmation
	quit    bool

	sdd      string // SDD subcommand running or waiting for approval (TUI-27)
	approval bool   // the next submitted line answers the SDD approval prompt (TUI-25)
}

// New builds the screen. Call SetSender before the program starts.
func New(b Backend) *Model {
	in := textinput.New()
	in.Prompt = "> "
	in.Focus()
	return &Model{backend: b, width: 80, height: 24, input: in}
}

// SetSender sets how background work delivers messages (Program.Send).
func (m *Model) SetSender(send func(tea.Msg)) { m.send = send }

// Wait blocks until an in-flight request has finished, including its history
// record (EXEC-8). Call it after the program exits.
func (m *Model) Wait() { m.inflight.Wait() }

// Quitting reports whether the screen asked to exit.
func (m *Model) Quitting() bool { return m.quit }

func (m *Model) post(msg tea.Msg) {
	if m.send != nil {
		m.send(msg)
	}
}

// Messages sent by background work.
type (
	loadedMsg struct {
		s   Startup
		err error
	}
	outMsg struct {
		kind lineKind
		text string
	}
	runDoneMsg struct{}
	toolMsg    struct{ lines []line }
	confirmMsg struct{ c *Confirmation }
	sddMsg     struct {
		r   SDDReply
		err error
	}
)

// Init opens the screen first and starts loading in the background (TUI-10).
func (m *Model) Init() tea.Cmd {
	m.add(kindInfo, "loading configuration and model metadata...")
	ctx, cancel := context.WithCancel(context.Background())
	m.loadCancel = cancel
	b := m.backend
	return func() tea.Msg {
		s, err := b.Load(ctx)
		return loadedMsg{s: s, err: err}
	}
}

// Update handles messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width <= 0 || msg.Height <= 0 {
			break
		}
		m.width, m.height = msg.Width, msg.Height
		m.input.Width = max(1, msg.Width-3)
	case tea.KeyMsg:
		return m.key(msg)
	case loadedMsg:
		m.onLoaded(msg)
	case outMsg:
		m.onOut(msg)
	case runDoneMsg:
		m.running, m.runCancel = false, nil
		m.out.closeLine()
	case toolMsg:
		m.addLines(msg.lines)
	case sddMsg:
		m.onSDD(msg)
	case confirmMsg:
		m.confirm = msg.c
		m.addLines(toLines(kindInfo, msg.c.Prompt))
	}
	return m, nil
}

func (m *Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.Type {
	case tea.KeyCtrlC:
		switch {
		case m.confirm != nil: // TUI-19: cancels the prompt, does not exit
			m.confirm = nil
			m.add(kindInfo, "cancelled")
		case m.approval: // TUI-25: Ctrl+C answers no
			m.add(kindEcho, "> no")
			return m, m.sddAnswer("no")
		case m.running: // TUI-5
			if m.runCancel != nil {
				m.runCancel()
			}
		default: // TUI-17, also during startup
			return m.exit()
		}
		return m, nil
	case tea.KeyCtrlD:
		if m.input.Value() == "" { // TUI-18
			return m.exit()
		}
		return m, nil
	case tea.KeyPgUp:
		m.out.scrollUp(m.width, m.pageSize())
		return m, nil
	case tea.KeyPgDown:
		m.out.scrollDown(m.pageSize())
		return m, nil
	case tea.KeyEnter:
		text := m.input.Value()
		m.input.SetValue("")
		return m.submit(text)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return m, cmd
}

func (m *Model) exit() (tea.Model, tea.Cmd) {
	if m.loadCancel != nil {
		m.loadCancel()
	}
	if m.runCancel != nil {
		m.runCancel()
	}
	m.quit = true
	return m, tea.Quit
}

func (m *Model) pageSize() int { return max(1, m.height-1) }

func (m *Model) onLoaded(l loadedMsg) {
	if l.err != nil {
		m.loadFailed = true // stay not-loaded: nothing may be dispatched without a configuration
		m.add(kindError, "startup failed: "+l.err.Error()+" (press Ctrl+C to exit)")
		return
	}
	m.loaded = true
	for _, w := range l.s.Warnings { // TUI-11 order
		m.addLines(toLines(kindWarning, w))
	}
	for _, p := range l.s.Problems {
		m.addLines(toLines(kindError, p.String()))
	}
	for _, w := range l.s.MetadataWarnings {
		m.addLines(toLines(kindWarning, w))
	}
	if l.s.Summary != "" {
		m.addLines(toLines(kindInfo, strings.TrimRight(l.s.Summary, "\n")))
	}
}

func (m *Model) onOut(o outMsg) {
	switch o.kind {
	case kindChunk:
		m.out.appendText(m.width, o.text)
	case kindError:
		m.out.closeLine()
		m.addLines(toLines(kindError, formatError(o.text)))
	default:
		m.out.closeLine()
		m.addLines(toLines(o.kind, o.text))
	}
}

// submit classifies a line by its first word only (TUI-2).
func (m *Model) submit(text string) (tea.Model, tea.Cmd) {
	line := strings.TrimLeftFunc(text, unicode.IsSpace)
	if strings.TrimSpace(line) == "" {
		return m, nil
	}
	if m.confirm != nil { // TUI-19: the answer is never dispatched as a command
		return m.answer(line)
	}
	if m.approval { // TUI-25: the answer is never dispatched as a command
		m.add(kindEcho, "> "+line)
		return m, m.sddAnswer(line)
	}
	m.add(kindEcho, "> "+text)
	if !m.loaded { // TUI-12
		if m.loadFailed {
			m.add(kindError, "startup failed, nothing can run; press Ctrl+C to exit")
		} else {
			m.add(kindError, "startup is in progress; try again when loading has finished")
		}
		return m, nil
	}
	first, rest := splitFirst(line)
	if isTool(first) {
		return m.tool(first, rest)
	}
	return m.routed(line)
}

func (m *Model) answer(line string) (tea.Model, tea.Cmd) {
	c := m.confirm
	m.confirm = nil
	m.add(kindEcho, "> "+line)
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		m.add(kindInfo, "cancelled")
		return m, nil
	}
	if m.running { // TUI-15: a request started after the prompt was asked
		m.add(kindError, "the confirmation is refused while a request is running; nothing was changed")
		return m, nil
	}
	return m, func() tea.Msg {
		lines, err := c.Run(context.Background())
		return toolMsg{lines: withErr(toLines(kindInfo, strings.Join(lines, "\n")), err)}
	}
}

func (m *Model) routed(line string) (tea.Model, tea.Cmd) {
	d := m.backend.Route(line)
	switch d.Kind {
	case routing.Routed:
	case routing.Refused:
		m.addLines(toLines(kindError, d.Message))
		return m, nil
	default:
		return m, nil
	}
	if m.sdd != "" { // TUI-27
		m.add(kindError, "/sdd "+m.sdd+" is active: routing requests are refused until it finishes")
		return m, nil
	}
	if m.running { // TUI-14
		m.add(kindError, "a request is already running")
		return m, nil
	}
	m.running = true
	ctx, cancel := context.WithCancel(context.Background())
	m.runCancel = cancel
	m.add(kindInfo, string(d.Model)+":") // model label (TUI-3)
	b, sink := m.backend, sink{post: m.post}
	m.inflight.Add(1)
	return m, func() tea.Msg {
		defer m.inflight.Done()
		b.Execute(ctx, d, sink)
		return runDoneMsg{}
	}
}

func (m *Model) tool(name, args string) (tea.Model, tea.Cmd) {
	b := m.backend
	switch name {
	case "/help":
		m.addLines(helpLines())
	case "/models":
		m.addLines(modelsLines(b.Config()))
	case "/suggest":
		return m, func() tea.Msg {
			s, err := b.Suggest(context.Background(), args)
			return toolMsg{lines: withErr(toRawLines(kindPlain, s), err)} // TUI-23: no decoration
		}
	case "/sdd":
		return m, m.sddStart(args)
	case "/refresh":
		if m.sdd != "" {
			m.add(kindError, "/refresh is refused while /sdd "+m.sdd+" is active")
			return m, nil
		}
		if m.running { // TUI-15, REFRESH-9
			m.add(kindError, "/refresh is refused while a request is running")
			return m, nil
		}
		return m, func() tea.Msg {
			o, err := b.Refresh(context.Background())
			return toolMsg{lines: refreshLines(o, err)}
		}
	case "/history":
		if m.sdd != "" && b.HistoryMutates(args) {
			m.add(kindError, "/history delete and /history clear are refused while /sdd "+m.sdd+" is active")
			return m, nil
		}
		if m.running && b.HistoryMutates(args) {
			m.add(kindError, "/history delete and /history clear are refused while a request is running")
			return m, nil
		}
		return m, func() tea.Msg {
			r, err := b.History(context.Background(), args)
			if err == nil && r.Confirm != nil {
				return confirmMsg{c: r.Confirm}
			}
			var ls []line
			for _, w := range r.Warnings {
				ls = append(ls, toLines(kindWarning, w)...)
			}
			ls = append(ls, toLines(kindInfo, strings.Join(r.Lines, "\n"))...)
			return toolMsg{lines: withErr(ls, err)}
		}
	}
	return m, nil
}

func (m *Model) add(k lineKind, text string) { m.addLines(toLines(k, text)) }
func (m *Model) addLines(ls []line)          { m.out.addAll(m.width, ls) }
func isTool(first string) bool {
	for _, t := range domain.ToolCommands {
		if first == t {
			return true
		}
	}
	return false
}

func splitFirst(s string) (string, string) {
	i := strings.IndexFunc(s, unicode.IsSpace)
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimLeftFunc(s[i:], unicode.IsSpace)
}

func withErr(ls []line, err error) []line {
	if err != nil && !errors.Is(err, context.Canceled) {
		ls = append(ls, toLines(kindError, err.Error())...)
	}
	return ls
}

// sink adapts execution.Output to screen messages. It is safe to call from
// the request goroutine.
type sink struct{ post func(tea.Msg) }

func (s sink) Chunk(t string)  { s.post(outMsg{kind: kindChunk, text: t}) }
func (s sink) Final(t string)  { s.post(outMsg{kind: kindPlain, text: t}) }
func (s sink) Notice(t string) { s.post(outMsg{kind: kindInfo, text: t}) }
func (s sink) Error(t string)  { s.post(outMsg{kind: kindError, text: t}) }

// sddStart dispatches a /sdd command. A model-using subcommand makes the screen
// busy until it finishes or its proposal is answered (TUI-27, SDD-40).
func (m *Model) sddStart(args string) tea.Cmd {
	if m.sdd != "" {
		m.add(kindError, "/sdd "+m.sdd+" is already running or waiting for approval")
		return nil
	}
	kind := m.backend.SDDKind(args)
	if m.running { // TUI-14
		m.add(kindError, "a request is already running")
		return nil
	}
	return m.sddRun(kind, func(ctx context.Context, out execution.Output) (SDDReply, error) {
		return m.backend.SDD(ctx, args, out)
	})
}

// sddAnswer sends the approval answer (SDD-49). It may stream again for `redo`.
func (m *Model) sddAnswer(line string) tea.Cmd {
	m.approval = false
	return m.sddRun(m.sdd, func(ctx context.Context, out execution.Output) (SDDReply, error) {
		return m.backend.SDDAnswer(ctx, line, out)
	})
}

func (m *Model) sddRun(kind string, run func(context.Context, execution.Output) (SDDReply, error)) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	if kind != "" {
		m.sdd = kind
	}
	m.running, m.runCancel = true, cancel
	sk := sink{post: m.post}
	m.inflight.Add(1)
	return func() tea.Msg {
		defer m.inflight.Done()
		r, err := run(ctx, sk)
		return sddMsg{r: r, err: err}
	}
}

func (m *Model) onSDD(msg sddMsg) {
	m.running, m.runCancel = false, nil
	m.out.closeLine()
	for _, l := range msg.r.Lines {
		m.addLines(toLines(kindInfo, l))
	}
	if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
		m.addLines(toLines(kindError, msg.err.Error()))
	}
	switch {
	case msg.r.Waiting: // TUI-25
		m.approval = true
		return
	case msg.r.Confirm != nil: // TUI-28
		m.confirm = msg.r.Confirm
		m.addLines(toLines(kindInfo, msg.r.Confirm.Prompt))
	}
	m.sdd, m.approval = "", false
}
