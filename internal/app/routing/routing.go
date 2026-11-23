// Package routing turns an explicit routing command into one model reference.
// It never imports provider code (ROUTE-8, ARCH-1).
package routing

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/domain"
)

// Kind says what the caller must do with the input.
type Kind int

const (
	// Refused: show Message, execute nothing.
	Refused Kind = iota
	// ToolCommand: handle Tool in the app, never send to a model (ROUTE-7).
	ToolCommand
	// Routed: send Prompt to Model (ROUTE-5, ROUTE-6).
	Routed
)

// Decision is the routing outcome. It holds a model reference only (ROUTE-8).
type Decision struct {
	Kind    Kind
	Tool    string                // set when Kind == ToolCommand
	Command domain.RoutingCommand // set when Kind == Routed
	Model   domain.ModelRef       // set when Kind == Routed
	Prompt  string                // set when Kind == Routed
	Message string                // set when Kind == Refused
}

// Route decides what to do with one user input. Requests are independent:
// no conversation context is used (ROUTE-9). The prompt is never inspected
// to infer a command and there is no default model (ROUTE-3).
func Route(cfg configuration.Config, input string) Decision {
	trimmed := strings.TrimLeftFunc(input, unicode.IsSpace)
	first, rest := splitFirst(trimmed)

	// Tool commands come first (ROUTE-7). Matching is case-sensitive (ROUTE-11).
	for _, t := range domain.ToolCommands {
		if first == t {
			return Decision{Kind: ToolCommand, Tool: t}
		}
	}

	if !strings.HasPrefix(first, "/") {
		return refuse(cfg, "input must start with a routing command")
	}
	cmd, ok := parseCommand(first)
	if !ok {
		return refuse(cfg, fmt.Sprintf("unknown command %q", first))
	}

	if reason, bad := checkUsable(cfg, cmd); bad {
		return refuse(cfg, reason)
	}

	prompt := strings.TrimSpace(rest)
	if prompt == "" {
		return Decision{Kind: Refused, Message: fmt.Sprintf("%s needs a prompt: type %s followed by your request.", cmd, cmd)}
	}
	// With several models, use the first listed (ROUTE-6, D-004).
	model := cfg.Strategies.Commands[string(cmd)].Models[0]
	return Decision{Kind: Routed, Command: cmd, Model: domain.ModelRef(model), Prompt: prompt}
}

// splitFirst returns the first whitespace-delimited token and the remainder.
func splitFirst(s string) (string, string) {
	i := strings.IndexFunc(s, unicode.IsSpace)
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i:]
}

func parseCommand(s string) (domain.RoutingCommand, bool) {
	for _, c := range domain.RoutingCommands {
		if string(c) == s {
			return c, true
		}
	}
	return "", false
}

// checkUsable returns a distinct reason when a command cannot be used (ROUTE-2, ROUTE-4).
func checkUsable(cfg configuration.Config, cmd domain.RoutingCommand) (string, bool) {
	name := string(cmd)
	if cfg.Strategies == nil {
		return fmt.Sprintf("%s is unavailable: strategies.json is missing or invalid.", name), true
	}
	a, ok := cfg.Strategies.Commands[name]
	if !ok || !a.Enabled {
		return fmt.Sprintf("%s is disabled in strategies.json.", name), true
	}
	if len(a.Models) == 0 {
		return fmt.Sprintf("%s has no model assigned in strategies.json.", name), true
	}
	if reason, bad := cfg.Unusable[name]; bad {
		return fmt.Sprintf("%s is unusable: %s.", name, reason), true
	}
	return "", false
}

// refuse builds a refusal that explains the problem and lists available commands (ROUTE-4).
func refuse(cfg configuration.Config, why string) Decision {
	return Decision{Kind: Refused, Message: fmt.Sprintf("%s. Available commands: %s.", strings.TrimSuffix(why, "."), availableList(cfg))}
}

func availableList(cfg configuration.Config) string {
	var out []string
	for _, c := range domain.RoutingCommands {
		if _, bad := checkUsable(cfg, c); !bad {
			out = append(out, string(c))
		}
	}
	if len(out) == 0 {
		return "none (check strategies.json)"
	}
	return strings.Join(out, ", ")
}

// Resolve turns a routing command into its model without a prompt (ROUTE-13).
// It applies the same rules as Route (ROUTE-2, ROUTE-4, ROUTE-6, CFG-10) and
// returns a model reference only (ROUTE-8). A refusal carries the ROUTE-4 reason.
func Resolve(cfg configuration.Config, cmd domain.RoutingCommand) Decision {
	if _, ok := parseCommand(string(cmd)); !ok {
		return Decision{Kind: Refused, Message: fmt.Sprintf("unknown command %q", string(cmd))}
	}
	if reason, bad := checkUsable(cfg, cmd); bad {
		return Decision{Kind: Refused, Message: reason}
	}
	model := cfg.Strategies.Commands[string(cmd)].Models[0]
	return Decision{Kind: Routed, Command: cmd, Model: domain.ModelRef(model)}
}
