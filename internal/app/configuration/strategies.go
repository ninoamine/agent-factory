package configuration

import (
	"fmt"
	"sort"

	"github.com/ninoamine/agent-factory/internal/domain"
)

// RoutingCommands are the valid keys of strategies.json "commands".
// Derived from the domain list so there is a single source (ROUTE-1).
var RoutingCommands = func() []string {
	out := make([]string, len(domain.RoutingCommands))
	for i, c := range domain.RoutingCommands {
		out[i] = string(c)
	}
	return out
}()

var validStrategies = map[string]bool{"cost": true, "speed": true, "capability": true}

// StrategiesFile is the typed content of strategies.json.
type StrategiesFile struct {
	Strategy string                       `json:"strategy"`
	Commands map[string]CommandAssignment `json:"commands"`
	// SDD optionally maps /sdd subcommands to routing commands (CFG-11, FILE-4).
	SDD map[string]string `json:"sdd,omitempty"`
}

// SDDSubcommands are the /sdd subcommands that use a model (CFG-11).
var SDDSubcommands = []string{"roadmap", "spec", "spec-validate", "implement", "spec-review", "review"}

// CommandAssignment maps one routing command to its models.
type CommandAssignment struct {
	Enabled bool     `json:"enabled"`
	Models  []string `json:"models"`
}

// Validate checks the content and returns every problem found.
func (s *StrategiesFile) Validate() []FieldError {
	var errs []FieldError
	if !validStrategies[s.Strategy] {
		errs = append(errs, FieldError{"strategy", fmt.Sprintf("%q is not one of cost, speed, capability", s.Strategy)})
	}
	names := make([]string, 0, len(s.Commands))
	for name := range s.Commands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !isRoutingCommand(name) {
			errs = append(errs, FieldError{"commands." + name, "unknown routing command"})
			continue
		}
		ms := s.Commands[name].Models
		if len(ms) > 1 {
			errs = append(errs, FieldError{"commands." + name + ".models", "at most one model per command (D-004)"})
		}
		for i, id := range ms {
			if id == "" {
				errs = append(errs, FieldError{fmt.Sprintf("commands.%s.models[%d]", name, i), "must not be empty"})
			}
		}
	}
	errs = append(errs, s.validateSDD()...)
	return errs
}

// validateSDD checks the optional sdd object (CFG-11). A mapped command that is
// disabled or unassigned is not an error here; it is refused at run time (SDD-38).
func (s *StrategiesFile) validateSDD() []FieldError {
	var errs []FieldError
	keys := make([]string, 0, len(s.SDD))
	for k := range s.SDD {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		known := false
		for _, n := range SDDSubcommands {
			known = known || n == k
		}
		if !known {
			errs = append(errs, FieldError{"sdd." + k, "unknown sdd subcommand"})
			continue
		}
		if !isRoutingCommand(s.SDD[k]) {
			errs = append(errs, FieldError{"sdd." + k, fmt.Sprintf("%q is not one of /fast, /code, /reason, /creative", s.SDD[k])})
		}
	}
	return errs
}

func isRoutingCommand(name string) bool {
	for _, c := range RoutingCommands {
		if c == name {
			return true
		}
	}
	return false
}
