package main

import (
	"context"
	"fmt"

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
	// registry builds the provider adapters from a configuration (PROV-10); may be nil in tests.
	registry interface{ Configure(configuration.Config) }

	manager *configuration.Manager
}

// Load loads the configuration, generates metadata if absent, and builds the
// manager that owns the active configuration (CFG, META-9).
func (b *backend) Load(ctx context.Context) (cli.Startup, error) {
	res, err := configuration.Load(ctx, b.reader, b.dir)
	if err != nil {
		return cli.Startup{}, err
	}
	b.configure(res.Config)
	s := cli.Startup{Warnings: res.Warnings, Problems: res.Problems, Summary: res.Config.Summary()}
	mres, _, err := b.metadata.GenerateIfAbsent(ctx, res.Config)
	if err != nil {
		s.MetadataWarnings = append(s.MetadataWarnings, fmt.Sprintf("could not generate %s: %v", configuration.MetadataFileName, err))
	} else {
		s.MetadataWarnings = append(s.MetadataWarnings, mres.Warnings...)
	}
	b.manager = configuration.NewManager(b.reader, b.regenerator(), res.Config)
	return s, nil
}

func (b *backend) configure(cfg configuration.Config) {
	if b.registry != nil {
		b.registry.Configure(cfg)
	}
}

// regenerator builds the adapters of the new configuration before metadata is
// fetched with them (PROV-10). If regeneration fails, the previous
// configuration stays active, so its adapters are restored (REFRESH-3).
func (b *backend) regenerator() configuration.MetadataRegenerator {
	regen := b.metadata.Regenerator()
	return func(ctx context.Context, cfg configuration.Config) ([]string, error) {
		b.configure(cfg)
		warns, err := regen(ctx, cfg)
		if err != nil {
			b.configure(b.manager.Active())
		}
		return warns, err
	}
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

func (b *backend) HistoryMutates(args string) bool { return history.Mutates(args) }

// History runs a /history command (HIST-5, HIST-6).
func (b *backend) History(ctx context.Context, args string) (cli.HistoryReply, error) {
	return b.history.Command(ctx, args)
}
