// Command agent-factory is the interactive terminal app (TUI).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/history"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
	"github.com/ninoamine/agent-factory/internal/app/strategies"
	"github.com/ninoamine/agent-factory/internal/cli"
	"github.com/ninoamine/agent-factory/internal/infra/providers"
	"github.com/ninoamine/agent-factory/internal/infra/storage"
)

// version is set at build time with -ldflags "-X main.version=<tag>" (DOC-11).
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("agent-factory", version)
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	dir, err := configuration.ResolveDir()
	if err != nil {
		return err
	}
	files, writer := storage.Files{}, storage.Writer{}
	registry := providers.NewRegistry() // adapters are built from the configuration (PROV-10)
	registry.Version = version
	hist := &history.Service{Store: storage.HistoryFile{Path: filepath.Join(dir, "history.jsonl"), Writer: writer}}

	b := &backend{
		dir:      dir,
		registry: registry,
		reader:   files,
		metadata: metadata.Service{Reader: files, Writer: writer, Fetcher: registry},
		exec:     execution.Service{Providers: registry, History: hist},
		history:  hist,
		suggest:  strategies.Service{Reader: files},
	}
	m := cli.New(b)
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.SetSender(p.Send)
	_, err = p.Run()
	m.Wait() // let an interrupted request write its history record (EXEC-8)
	return err
}
