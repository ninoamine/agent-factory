package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/history"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
	"github.com/ninoamine/agent-factory/internal/app/routing"
	"github.com/ninoamine/agent-factory/internal/app/strategies"
	"github.com/ninoamine/agent-factory/internal/cli"
)

// backend wires the application services to the screen (cli.Backend).
// It only connects them; the logic stays in the application layer (ARCH-3).
type backend struct {
	dir      string
	reader   configuration.FileReader
	metadata metadata.Service
	exec     execution.Service
	history  *history.Service
	suggest  strategies.Service

	manager *configuration.Manager
}

// Load loads the configuration, generates metadata if absent, and builds the
// manager that owns the active configuration (CFG, META-9).
func (b *backend) Load(ctx context.Context) (cli.Startup, error) {
	res, err := configuration.Load(ctx, b.reader, b.dir)
	if err != nil {
		return cli.Startup{}, err
	}
	s := cli.Startup{Warnings: res.Warnings, Problems: res.Problems, Summary: res.Config.Summary()}
	mres, _, err := b.metadata.GenerateIfAbsent(ctx, res.Config)
	if err != nil {
		s.MetadataWarnings = append(s.MetadataWarnings, fmt.Sprintf("could not generate %s: %v", configuration.MetadataFileName, err))
	} else {
		s.MetadataWarnings = append(s.MetadataWarnings, mres.Warnings...)
	}
	b.manager = configuration.NewManager(b.reader, b.metadata.Regenerator(), res.Config)
	return s, nil
}

func (b *backend) Config() configuration.Config { return b.manager.Active() }

func (b *backend) Route(input string) routing.Decision {
	return routing.Route(b.manager.Active(), input)
}

func (b *backend) Execute(ctx context.Context, d routing.Decision, out execution.Output) {
	cfg, release := b.manager.BeginRequest() // REFRESH-9
	defer release()
	b.exec.Run(ctx, cfg, d, out)
}

func (b *backend) Suggest(ctx context.Context, args string) (string, error) {
	return b.suggest.Run(ctx, b.dir, args)
}

func (b *backend) Refresh(ctx context.Context) (configuration.RefreshOutcome, error) {
	return b.manager.Refresh(ctx)
}

// historyArgs reads the first word of the /history arguments.
func historyArgs(args string) (sub, id string) {
	f := strings.Fields(args)
	if len(f) > 0 {
		sub = f[0]
	}
	if len(f) > 1 {
		id = f[1]
	}
	return sub, id
}

func (b *backend) HistoryMutates(args string) bool {
	sub, _ := historyArgs(args)
	return sub == "delete" || sub == "clear"
}

func (b *backend) History(ctx context.Context, args string) (cli.HistoryReply, error) {
	sub, id := historyArgs(args)
	switch sub {
	case "":
		entries, warns, err := b.history.List(ctx)
		if err != nil {
			return cli.HistoryReply{}, err
		}
		r := cli.HistoryReply{Warnings: warns}
		for _, e := range entries {
			r.Lines = append(r.Lines, formatEntry(e))
		}
		if len(r.Lines) == 0 {
			r.Lines = []string{"history is empty"}
		}
		return r, nil
	case "delete":
		if id == "" {
			return cli.HistoryReply{Lines: []string{"usage: /history delete <id>"}}, nil
		}
		if err := b.history.Delete(ctx, id); err != nil {
			if errors.Is(err, history.ErrNotFound) {
				return cli.HistoryReply{Lines: []string{"no history entry with id " + id}}, nil
			}
			return cli.HistoryReply{}, err
		}
		return cli.HistoryReply{Lines: []string{"deleted " + id}}, nil
	case "clear":
		return cli.HistoryReply{Confirm: &cli.Confirmation{
			Prompt: "Delete all history entries? Type y or yes to confirm.",
			Run: func(ctx context.Context) ([]string, error) {
				if err := b.history.Clear(ctx); err != nil {
					return nil, err
				}
				return []string{"history cleared"}, nil
			},
		}}, nil
	}
	return cli.HistoryReply{Lines: []string{"usage: /history | /history delete <id> | /history clear"}}, nil
}

func formatEntry(e history.Entry) string {
	n := func(p *int) string {
		if p == nil {
			return "unavailable"
		}
		return fmt.Sprint(*p)
	}
	return fmt.Sprintf("%s | %s | %s | %s | in: %s | out: %s | %s",
		e.ID, e.Timestamp, e.Command, e.Model, n(e.InputTokens), n(e.OutputTokens), e.Status)
}
