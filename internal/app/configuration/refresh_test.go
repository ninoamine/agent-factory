package configuration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/infra/storage"
)

const otherModels = `{"providers":{"p":{"apiKey":"sk-SECRET-123"}},"models":[{"id":"p/x","provider":"p"},{"id":"p/y","provider":"p"}]}`
const otherStrat = `{"strategy":"cost","commands":{"/fast":{"enabled":true,"models":["p/y"]}}}`

func newManager(t *testing.T, regen configuration.MetadataRegenerator) (*configuration.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "models.json", goodModels, 0o600)
	write(t, dir, "strategies.json", goodStrat, 0o600)
	res := load(t, dir)
	return configuration.NewManager(storage.Files{}, regen, res.Config), dir
}

func fastModel(m *configuration.Manager) string {
	return m.Active().Strategies.Commands["/fast"].Models[0]
}

func TestREFRESH1_3_ValidEditsApplied(t *testing.T) {
	t.Run("REFRESH-1 REFRESH-3 valid edits are active after refresh", func(t *testing.T) {
		m, dir := newManager(t, nil)
		write(t, dir, "models.json", otherModels, 0o600)
		write(t, dir, "strategies.json", otherStrat, 0o600)
		out, err := m.Refresh(context.Background())
		if err != nil || !out.Applied {
			t.Fatalf("out=%+v err=%v", out, err)
		}
		if fastModel(m) != "p/y" || m.Active().Strategies.Strategy != "cost" {
			t.Errorf("new configuration not active: %+v", m.Active().Strategies)
		}
	})
}

func TestREFRESH2_RegeneratesMetadata(t *testing.T) {
	t.Run("REFRESH-2 regenerates metadata with the new configuration", func(t *testing.T) {
		var seen int
		m, dir := newManager(t, func(_ context.Context, cfg configuration.Config) ([]string, error) {
			seen = len(cfg.Models.Models)
			return nil, nil
		})
		write(t, dir, "models.json", otherModels, 0o600)
		if _, err := m.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
		if seen != 2 {
			t.Errorf("regenerator saw %d models, want 2", seen)
		}
	})
	t.Run("REFRESH-2 metadata is not regenerated when validation fails", func(t *testing.T) {
		called := false
		m, dir := newManager(t, func(context.Context, configuration.Config) ([]string, error) {
			called = true
			return nil, nil
		})
		write(t, dir, "strategies.json", `{bad`, 0o600)
		if _, err := m.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
		if called {
			t.Error("regenerator ran for an invalid configuration")
		}
	})
}

func TestREFRESH4_InvalidKeepsPrevious(t *testing.T) {
	t.Run("REFRESH-4 broken strategies.json keeps old routing and shows the error", func(t *testing.T) {
		m, dir := newManager(t, nil)
		write(t, dir, "models.json", otherModels, 0o600) // valid, must not be half-applied (REFRESH-3)
		write(t, dir, "strategies.json", `{"strategy":"nope","commands":{}}`, 0o600)
		out, err := m.Refresh(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if out.Applied || len(out.Problems) == 0 {
			t.Fatalf("out=%+v", out)
		}
		p := out.Problems[0]
		if p.File != "strategies.json" || p.Path != "strategy" || p.Reason == "" {
			t.Errorf("problem lacks file/field/reason: %+v", p)
		}
		if fastModel(m) != "p/x" || len(m.Active().Models.Models) != 1 {
			t.Error("previous configuration was not kept intact")
		}
		if !strings.Contains(out.String(), "previous configuration stays active") {
			t.Errorf("message: %s", out)
		}
	})
	t.Run("REFRESH-4 errors are reported per file", func(t *testing.T) {
		m, dir := newManager(t, nil)
		write(t, dir, "models.json", `{`, 0o600)
		write(t, dir, "strategies.json", `{`, 0o600)
		out, _ := m.Refresh(context.Background())
		files := map[string]bool{}
		for _, p := range out.Problems {
			files[p.File] = true
		}
		if !files["models.json"] || !files["strategies.json"] {
			t.Errorf("problems: %+v", out.Problems)
		}
	})
}

func TestREFRESH5_MetadataWarningDoesNotFail(t *testing.T) {
	t.Run("REFRESH-5 fetch warning is shown and refresh succeeds", func(t *testing.T) {
		m, _ := newManager(t, func(context.Context, configuration.Config) ([]string, error) {
			return []string{`could not fetch metadata for model "p/x"`}, nil
		})
		out, err := m.Refresh(context.Background())
		if err != nil || !out.Applied {
			t.Fatalf("out=%+v err=%v", out, err)
		}
		if !strings.Contains(out.String(), "could not fetch metadata") {
			t.Errorf("warning not shown: %s", out)
		}
	})
	t.Run("REFRESH-5 a metadata write error keeps the previous configuration", func(t *testing.T) {
		m, dir := newManager(t, func(context.Context, configuration.Config) ([]string, error) {
			return nil, errors.New("disk full")
		})
		write(t, dir, "strategies.json", otherStrat, 0o600)
		if _, err := m.Refresh(context.Background()); err == nil {
			t.Fatal("expected error")
		}
		if fastModel(m) != "p/x" {
			t.Error("configuration swapped despite error")
		}
	})
}

func TestREFRESH6_SummaryAndWarnings(t *testing.T) {
	t.Run("REFRESH-6 success shows summary without keys and missing-file warnings", func(t *testing.T) {
		m, dir := newManager(t, nil) // models-metadata.json is absent
		out, err := m.Refresh(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		s := out.String()
		if !strings.Contains(s, "Models (1): p/x") || !strings.Contains(s, filepath.Join(dir, "models-metadata.json")) {
			t.Errorf("missing summary or warning:\n%s", s)
		}
		if strings.Contains(s, "sk-SECRET-123") {
			t.Error("API key leaked (SEC-1)")
		}
	})
}

func TestREFRESH7_NoEffectWithoutRefresh(t *testing.T) {
	t.Run("REFRESH-7 edits change nothing until refresh", func(t *testing.T) {
		m, dir := newManager(t, nil)
		write(t, dir, "strategies.json", otherStrat, 0o600)
		write(t, dir, "models.json", `{broken`, 0o600)
		if fastModel(m) != "p/x" {
			t.Error("active configuration changed without /refresh")
		}
		cfg, release := m.BeginRequest()
		release()
		if cfg.Strategies.Commands["/fast"].Models[0] != "p/x" {
			t.Error("request saw an unrefreshed edit")
		}
	})
}

func TestREFRESH8_NoWritesToUserFiles(t *testing.T) {
	t.Run("REFRESH-8 refresh never writes models.json or strategies.json", func(t *testing.T) {
		m, dir := newManager(t, nil)
		write(t, dir, "strategies.json", `{bad`, 0o600)
		want := map[string]string{}
		for _, n := range []string{"models.json", "strategies.json"} {
			b, _ := os.ReadFile(filepath.Join(dir, n))
			want[n] = string(b)
		}
		for i := 0; i < 2; i++ {
			if _, err := m.Refresh(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		for n, w := range want {
			b, _ := os.ReadFile(filepath.Join(dir, n))
			if string(b) != w {
				t.Errorf("%s was modified", n)
			}
		}
		// A missing file is not created either.
		if err := os.Remove(filepath.Join(dir, "strategies.json")); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "strategies.json")); !os.IsNotExist(err) {
			t.Error("strategies.json was created")
		}
	})
}

func TestREFRESH9_RefusedWhileRequestRuns(t *testing.T) {
	t.Run("REFRESH-9 refused during a request, allowed after it finishes", func(t *testing.T) {
		m, dir := newManager(t, nil)
		write(t, dir, "strategies.json", otherStrat, 0o600)
		cfg, release := m.BeginRequest()
		_, err := m.Refresh(context.Background())
		if !errors.Is(err, configuration.ErrRequestRunning) {
			t.Fatalf("err=%v", err)
		}
		if fastModel(m) != "p/x" || cfg.Strategies.Commands["/fast"].Models[0] != "p/x" {
			t.Error("configuration changed while a request was running")
		}
		release()
		release() // releasing twice must not go negative
		out, err := m.Refresh(context.Background())
		if err != nil || !out.Applied || fastModel(m) != "p/y" {
			t.Fatalf("out=%+v err=%v", out, err)
		}
	})
}

func TestREFRESH_Cancellation(t *testing.T) {
	t.Run("REFRESH-3 cancelled context returns an error and keeps the configuration", func(t *testing.T) {
		m, dir := newManager(t, nil)
		write(t, dir, "strategies.json", otherStrat, 0o600)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := m.Refresh(ctx); err == nil {
			t.Fatal("expected error")
		}
		if fastModel(m) != "p/x" {
			t.Error("configuration swapped on cancellation")
		}
	})
}
