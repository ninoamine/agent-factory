package configuration

import (
	"fmt"
	"sort"
)

// RoutingCommands are the valid keys of strategies.json "commands".
var RoutingCommands = []string{"/fast", "/code", "/reason", "/creative"}

var validStrategies = map[string]bool{"cost": true, "speed": true, "capability": true}

// StrategiesFile is the typed content of strategies.json.
type StrategiesFile struct {
	Strategy string                       `json:"strategy"`
	Commands map[string]CommandAssignment `json:"commands"`
}

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
