package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/domain"
)

type lineKind int

const (
	kindPlain   lineKind = iota // model and tool output, shown as is
	kindInfo                    // loading line, summary, notices
	kindEcho                    // the user's own submitted line
	kindWarning                 // prefixed `warning: ` (TUI-6)
	kindError                   // prefixed `error: ` (TUI-6)
	kindChunk                   // streamed text (never stored as a line kind)
)

type line struct {
	kind lineKind
	text string
}

// toLines splits text into lines, prefixing every warning and error line (TUI-6).
func toLines(k lineKind, text string) []line {
	if text == "" {
		return nil
	}
	prefix := map[lineKind]string{kindWarning: "warning: ", kindError: "error: "}[k]
	parts := strings.Split(text, "\n")
	out := make([]line, len(parts))
	for i, p := range parts {
		out[i] = line{kind: k, text: prefix + p}
	}
	return out
}

func toRawLines(k lineKind, text string) []line { return toLines(k, text) }

var execErrorRe = regexp.MustCompile(`(?s)^error \(([a-z_]+)\): (.*)`)

// formatError turns execution's `error (kind): detail` into `kind: detail`
// (the screen adds the `error: ` prefix) and drops a repeated kind.
func formatError(s string) string {
	m := execErrorRe.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	kind, rest := m[1], m[2]
	return kind + ": " + strings.TrimPrefix(rest, kind+": ")
}

// output holds the output area: logical lines plus the scroll position.
type output struct {
	lines  []line
	open   bool // the last line is a streamed line still being written
	scroll int  // wrapped lines hidden below the view; 0 means at the bottom
}

func (o *output) addAll(width int, ls []line) {
	if len(ls) == 0 {
		return
	}
	o.mutate(width, func() {
		o.open = false
		o.lines = append(o.lines, ls...)
	})
}

func (o *output) closeLine() { o.open = false }

// appendText adds streamed text to the current line (TUI-4).
func (o *output) appendText(width int, text string) {
	if text == "" {
		return
	}
	o.mutate(width, func() {
		parts := strings.Split(text, "\n")
		for i, p := range parts {
			last := i == len(parts)-1
			if last && p == "" && i > 0 {
				o.open = false
				return
			}
			if o.open {
				o.lines[len(o.lines)-1].text += p
			} else {
				o.lines = append(o.lines, line{kind: kindPlain, text: p})
			}
			o.open = last
		}
	})
}

// mutate changes the lines and keeps the view still when it is scrolled up (TUI-20).
func (o *output) mutate(width int, change func()) {
	if o.scroll == 0 {
		change()
		return
	}
	before := len(o.wrapped(width))
	change()
	o.scroll += len(o.wrapped(width)) - before
}

func (o *output) scrollUp(width, page int) {
	limit := max(0, len(o.wrapped(width))-page)
	o.scroll = min(o.scroll+page, limit)
}

func (o *output) scrollDown(page int) { o.scroll = max(0, o.scroll-page) }

// wrapped soft-wraps every line to the width without adding anything (TUI-22, TUI-23).
// It breaks at spaces first, so a JSON line breaks between tokens (still valid
// JSON when copied); only a word longer than the width is split.
func (o *output) wrapped(width int) []line {
	var out []line
	for _, l := range o.lines {
		if l.text == "" {
			out = append(out, l)
			continue
		}
		for _, w := range strings.Split(ansi.Wrap(l.text, max(1, width), ""), "\n") {
			out = append(out, line{kind: l.kind, text: w})
		}
	}
	return out
}

var (
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	dimStyle  = lipgloss.NewStyle().Faint(true)
)

func (l line) render() string {
	switch l.kind {
	case kindWarning:
		return warnStyle.Render(l.text)
	case kindError:
		return errStyle.Render(l.text)
	case kindEcho:
		return dimStyle.Render(l.text)
	}
	return l.text
}

// View draws the output area above a one-line input box (TUI-1).
func (m *Model) View() string {
	h := m.pageSize()
	rl := m.out.wrapped(m.width)
	scroll := min(m.out.scroll, max(0, len(rl)-h))
	end := len(rl) - scroll
	start := max(0, end-h)
	rows := make([]string, 0, h)
	for _, l := range rl[start:end] {
		rows = append(rows, l.render())
	}
	for len(rows) < h {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n") + "\n" + m.input.View()
}

var routingHelp = map[domain.RoutingCommand]string{
	domain.CmdFast:     "answer quickly",
	domain.CmdCode:     "coding tasks",
	domain.CmdReason:   "reasoning tasks",
	domain.CmdCreative: "creative writing",
}

var toolHelp = map[string]string{
	"/refresh": "reload models.json and strategies.json, refresh metadata",
	"/suggest": "suggest a strategies.json: /suggest <cost|speed|capability>",
	"/models":  "list configured models and their commands",
	"/history": "list runs; /history delete <id>; /history clear",
	"/help":    "show this help",
}

// helpLines lists all tool commands and routing commands (TUI-8).
func helpLines() []line {
	var b strings.Builder
	b.WriteString("Tool commands:\n")
	for _, t := range domain.ToolCommands {
		fmt.Fprintf(&b, "  %-10s %s\n", t, toolHelp[t])
	}
	b.WriteString("Routing commands (start a request with one, then your prompt):\n")
	for _, c := range domain.RoutingCommands {
		fmt.Fprintf(&b, "  %-10s %s\n", c, routingHelp[c])
	}
	return toLines(kindInfo, strings.TrimRight(b.String(), "\n"))
}

// modelsLines lists each configured model with its id, provider and assigned
// routing commands (TUI-9). It never reads API keys (TUI-7).
func modelsLines(cfg configuration.Config) []line {
	if cfg.Models == nil || len(cfg.Models.Models) == 0 {
		return toLines(kindInfo, "no models configured")
	}
	var out []string
	for _, m := range cfg.Models.Models {
		cmds := cfg.AssignedCommands(m.ID)
		assigned := "none"
		if len(cmds) > 0 {
			assigned = strings.Join(cmds, ", ")
		}
		out = append(out, fmt.Sprintf("%s | provider: %s | commands: %s", m.ID, m.Provider, assigned))
	}
	return toLines(kindInfo, strings.Join(out, "\n"))
}

// refreshLines renders a /refresh outcome (REFRESH-4, REFRESH-6).
func refreshLines(o configuration.RefreshOutcome, err error) []line {
	if err != nil {
		return toLines(kindError, err.Error())
	}
	var out []line
	if !o.Applied {
		out = append(out, toLines(kindError, "refresh failed: the previous configuration stays active")...)
		for _, p := range o.Problems {
			out = append(out, toLines(kindError, p.String())...)
		}
	}
	for _, w := range o.Warnings {
		out = append(out, toLines(kindWarning, w)...)
	}
	if o.Applied {
		out = append(out, toLines(kindInfo, "refresh done")...)
		out = append(out, toLines(kindInfo, strings.TrimRight(o.Summary, "\n"))...)
	}
	return out
}
