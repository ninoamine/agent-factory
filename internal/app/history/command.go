package history

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Confirmation is a question to ask before a destructive action (HIST-6).
type Confirmation struct {
	Prompt string
	Run    func(ctx context.Context) ([]string, error)
}

// Reply is the outcome of a /history command, ready to display.
type Reply struct {
	Lines    []string
	Warnings []string
	Confirm  *Confirmation // set for `clear`
}

func parseArgs(args string) (sub, id string) {
	f := strings.Fields(args)
	if len(f) > 0 {
		sub = f[0]
	}
	if len(f) > 1 {
		id = f[1]
	}
	return sub, id
}

// Mutates reports whether the /history arguments delete data.
func Mutates(args string) bool {
	sub, _ := parseArgs(args)
	return sub == "delete" || sub == "clear"
}

// Command runs `/history [delete <id> | clear]` (HIST-5, HIST-6).
func (s *Service) Command(ctx context.Context, args string) (Reply, error) {
	sub, id := parseArgs(args)
	switch sub {
	case "":
		entries, warns, err := s.List(ctx)
		if err != nil {
			return Reply{}, err
		}
		r := Reply{Warnings: warns}
		for _, e := range entries {
			r.Lines = append(r.Lines, FormatEntry(e))
		}
		if len(r.Lines) == 0 {
			r.Lines = []string{"history is empty"}
		}
		return r, nil
	case "delete":
		if id == "" {
			return Reply{Lines: []string{"usage: /history delete <id>"}}, nil
		}
		if err := s.Delete(ctx, id); err != nil {
			if errors.Is(err, ErrNotFound) {
				return Reply{Lines: []string{"no history entry with id " + id}}, nil
			}
			return Reply{}, err
		}
		return Reply{Lines: []string{"deleted " + id}}, nil
	case "clear":
		return Reply{Confirm: &Confirmation{
			Prompt: "Delete all history entries? Type y or yes to confirm.",
			Run: func(ctx context.Context) ([]string, error) {
				if err := s.Clear(ctx); err != nil {
					return nil, err
				}
				return []string{"history cleared"}, nil
			},
		}}, nil
	}
	return Reply{Lines: []string{"usage: /history | /history delete <id> | /history clear"}}, nil
}

// FormatEntry renders one entry with its id (HIST-5).
func FormatEntry(e Entry) string {
	n := func(p *int) string {
		if p == nil {
			return "unavailable"
		}
		return fmt.Sprint(*p)
	}
	return fmt.Sprintf("%s | %s | %s | %s | in: %s | out: %s | %s",
		e.ID, e.Timestamp, e.Command, e.Model, n(e.InputTokens), n(e.OutputTokens), e.Status)
}
