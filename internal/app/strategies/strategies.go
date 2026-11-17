// Package strategies builds a suggested strategies.json for the user to copy.
// It reads models-metadata.json and never writes a file or runs a request
// (STRAT-9, ARCH-4).
package strategies

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
	"github.com/ninoamine/agent-factory/internal/domain"
)

// Valid strategy names.
var Names = []string{"cost", "speed", "capability"}

// capabilityTag maps each routing command to the capability tag it needs (STRAT-12).
var capabilityTag = map[domain.RoutingCommand]string{
	domain.CmdFast:     "speed",
	domain.CmdCode:     "code",
	domain.CmdReason:   "reasoning",
	domain.CmdCreative: "creative",
}

// speedRank implements STRAT-13: higher is faster; unknown values are absent.
var speedRank = map[string]int{"slow": 1, "medium": 2, "fast": 3}

// Usage is shown for a missing or invalid argument (STRAT-2).
func Usage() string {
	return "Usage: /suggest <cost|speed|capability>\n" +
		"  cost        cheapest model where pricing is known\n" +
		"  speed       fastest model where speed is known\n" +
		"  capability  best match per command from capability tags"
}

// ParseArg returns the strategy for a /suggest argument. It never guesses (STRAT-2).
func ParseArg(arg string) (string, bool) {
	a := strings.ToLower(strings.TrimSpace(arg))
	for _, n := range Names {
		if a == n {
			return n, true
		}
	}
	return "", false
}

// Assignment is the suggestion for one routing command.
type Assignment struct {
	Command string
	Model   string // empty: unassigned (STRAT-7)
	Reason  string
}

// Suggestion is the result of one /suggest.
type Suggestion struct {
	Strategy    string
	Assignments []Assignment
}

// File builds the strategies.json content. At most one model per command
// (D-004); unassigned commands are disabled with an empty list (STRAT-7).
func (s Suggestion) File() configuration.StrategiesFile {
	f := configuration.StrategiesFile{Strategy: s.Strategy, Commands: map[string]configuration.CommandAssignment{}}
	for _, a := range s.Assignments {
		ca := configuration.CommandAssignment{Models: []string{}}
		if a.Model != "" {
			ca.Enabled, ca.Models = true, []string{a.Model}
		}
		f.Commands[a.Command] = ca
	}
	return f
}

// Render returns the text to display: JSON block, reasons outside it
// (STRAT-6), then the paste instruction (STRAT-10).
func (s Suggestion) Render(dir string) (string, error) {
	f := s.File()
	if errs := f.Validate(); len(errs) > 0 { // STRAT-11
		return "", fmt.Errorf("internal error: suggestion fails strategies.json validation: %s: %s", errs[0].Path, errs[0].Reason)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encoding suggestion: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Suggested strategies.json (%s):\n\n```json\n%s\n```\n\nReasons:\n", s.Strategy, data)
	for _, a := range s.Assignments {
		if a.Model == "" {
			fmt.Fprintf(&b, "- %s: unassigned. %s\n", a.Command, a.Reason)
		} else {
			fmt.Fprintf(&b, "- %s: %s. %s\n", a.Command, a.Model, a.Reason)
		}
	}
	fmt.Fprintf(&b, "\nPaste into %s, then run /refresh.\n", filepath.Join(dir, configuration.StrategiesFileName))
	return b.String(), nil
}

// Suggest builds a suggestion from metadata entries. Pure: no I/O.
func Suggest(strategy string, entries []metadata.Entry) (Suggestion, error) {
	if _, ok := ParseArg(strategy); !ok {
		return Suggestion{}, fmt.Errorf("unknown strategy %q", strategy)
	}
	sug := Suggestion{Strategy: strategy}
	var usable []metadata.Entry // META-5: models without metadata are never suggested
	for _, e := range entries {
		if e.HasMetadata() {
			usable = append(usable, e)
		}
	}
	for _, c := range domain.RoutingCommands {
		a := Assignment{Command: string(c)}
		switch strategy {
		case "cost":
			a.Model, a.Reason = pickCost(usable)
		case "speed":
			a.Model, a.Reason = pickSpeed(usable)
		default:
			a.Model, a.Reason = pickCapability(usable, capabilityTag[c])
		}
		sug.Assignments = append(sug.Assignments, a)
	}
	return sug, nil
}

// estimatedCost needs both prices; otherwise pricing is not usable (STRAT-3).
func estimatedCost(e metadata.Entry) (float64, bool) {
	if e.Pricing == nil || e.Pricing.InputPer1M == nil || e.Pricing.OutputPer1M == nil {
		return 0, false
	}
	return *e.Pricing.InputPer1M + *e.Pricing.OutputPer1M, true
}

func pickCost(entries []metadata.Entry) (string, string) {
	best, bestCost, found := "", 0.0, false
	for _, e := range entries {
		if c, ok := estimatedCost(e); ok && (!found || c < bestCost) {
			best, bestCost, found = e.ID, c, true
		}
	}
	if !found {
		return "", "No model has usable pricing (input and output price), so no cost advantage can be claimed."
	}
	return best, fmt.Sprintf("Lowest estimated cost among models with known pricing (%.4g USD per 1M tokens, input plus output).", bestCost)
}

func pickSpeed(entries []metadata.Entry) (string, string) {
	best, bestRank := "", 0
	for _, e := range entries {
		if r := speedRank[strings.ToLower(strings.TrimSpace(e.Speed))]; r > bestRank {
			best, bestRank = e.ID, r
		}
	}
	if best == "" {
		return "", "No model has a speed of fast, medium or slow, so no ranking is possible."
	}
	return best, fmt.Sprintf("Fastest declared speed (%q) among models with known speed.", speedName(bestRank))
}

func speedName(rank int) string {
	for n, r := range speedRank {
		if r == rank {
			return n
		}
	}
	return ""
}

func pickCapability(entries []metadata.Entry, tag string) (string, string) {
	for _, e := range entries {
		for _, c := range e.Capabilities {
			if strings.EqualFold(strings.TrimSpace(c), tag) {
				return e.ID, fmt.Sprintf("First model with the %q capability tag.", tag)
			}
		}
	}
	return "", fmt.Sprintf("No known model has the %q capability tag.", tag)
}

// validateEntries checks untrusted metadata file content and returns the first problem.
func validateEntries(entries []metadata.Entry) string {
	for i, e := range entries {
		if strings.TrimSpace(e.ID) == "" {
			return fmt.Sprintf("models[%d].id is empty", i)
		}
		api := metadata.APIMetadata{Capabilities: e.Capabilities, Speed: e.Speed, Pricing: e.Pricing}
		if err := api.Validate(); err != nil {
			return fmt.Sprintf("model %q: %v", e.ID, err)
		}
		if e.Sources != nil {
			for _, s := range []metadata.Source{e.Sources.Capabilities, e.Sources.Speed, e.Sources.Pricing} {
				if s != "" && s != metadata.SourceAPI && s != metadata.SourceUser {
					return fmt.Sprintf("model %q: unknown source %q", e.ID, s)
				}
			}
		}
	}
	return ""
}

// Service reads models-metadata.json (read-only) and produces the display text.
type Service struct {
	Reader configuration.FileReader
}

// Run handles `/suggest <arg>` and returns the text to display (STRAT-1, STRAT-2).
// It writes nothing (STRAT-9).
func (s Service) Run(ctx context.Context, dir, arg string) (string, error) {
	strategy, ok := ParseArg(arg)
	if !ok {
		return Usage(), nil
	}
	path := filepath.Join(dir, configuration.MetadataFileName)
	data, err := s.Reader.Read(ctx, path)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return "", cerr
		}
		return fmt.Sprintf("Cannot read %s: %v. Create or fix models.json, then run /refresh, and try /suggest again.", path, err), nil
	}
	var f metadata.File
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return fmt.Sprintf("%s is not valid (%v). Run /refresh to regenerate it, then try /suggest again.", path, err), nil
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Sprintf("%s has unexpected trailing content. Run /refresh to regenerate it, then try /suggest again.", path), nil
	}
	if msg := validateEntries(f.Models); msg != "" { // CONV-5, SEC-3
		return fmt.Sprintf("%s is not valid (%s). Run /refresh to regenerate it, then try /suggest again.", path, msg), nil
	}
	sug, err := Suggest(strategy, f.Models)
	if err != nil {
		return "", err
	}
	return sug.Render(dir)
}
