package configuration_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/infra/storage"
)

const goodModels = `{"providers":{"p":{"apiKey":"sk-SECRET-123"}},"models":[{"id":"p/x","provider":"p"}]}`
const goodStrat = `{"strategy":"capability","commands":{"/fast":{"enabled":true,"models":["p/x"]}}}`

func write(t *testing.T, dir, name, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func load(t *testing.T, dir string) configuration.Result {
	t.Helper()
	res, err := configuration.Load(context.Background(), storage.Files{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestCFG1ResolveDir(t *testing.T) {
	t.Run("CFG-1 env var wins", func(t *testing.T) {
		t.Setenv("AGENT_FACTORY_HOME", "/custom")
		got, _ := configuration.ResolveDir()
		if got != "/custom" {
			t.Fatalf("got %s", got)
		}
	})
	t.Run("CFG-1 default is ~/.agent-factory", func(t *testing.T) {
		t.Setenv("AGENT_FACTORY_HOME", "")
		t.Setenv("HOME", "/home/u")
		got, _ := configuration.ResolveDir()
		if got != "/home/u/.agent-factory" {
			t.Fatalf("got %s", got)
		}
	})
}

func TestCFG2_3_4MissingFiles(t *testing.T) {
	dir := t.TempDir()
	res := load(t, dir)
	if len(res.Warnings) != 3 {
		t.Fatalf("want 3 warnings, got %v", res.Warnings)
	}
	for i, name := range []string{"models.json", "strategies.json", "models-metadata.json"} {
		if !strings.Contains(res.Warnings[i], filepath.Join(dir, name)) {
			t.Errorf("CFG-2 warning %d lacks full path: %s", i, res.Warnings[i])
		}
	}
	t.Run("CFG-3 models.json guidance, not created", func(t *testing.T) {
		if !strings.Contains(res.Warnings[0], "no model is usable") {
			t.Error(res.Warnings[0])
		}
		if _, err := os.Stat(filepath.Join(dir, "models.json")); err == nil {
			t.Error("file was created")
		}
	})
	t.Run("CFG-4 strategies.json guidance, not created", func(t *testing.T) {
		w := res.Warnings[1]
		if !strings.Contains(w, "/suggest") || !strings.Contains(w, "/refresh") {
			t.Error(w)
		}
		if _, err := os.Stat(filepath.Join(dir, "strategies.json")); err == nil {
			t.Error("file was created")
		}
	})
	t.Run("AC1 CFG-2 CFG-3 CFG-4 empty dir has no problems", func(t *testing.T) {
		if len(res.Problems) != 0 {
			t.Error(res.Problems)
		}
	})
}

func TestCFG5Validation(t *testing.T) {
	cases := []struct {
		name, file, content, path string
	}{
		{"malformed json", "models.json", `{"providers":`, "$"},
		{"unknown field", "models.json", `{"providers":{},"models":[],"extra":1}`, "extra"},
		{"wrong type", "models.json", `{"providers":{},"models":"x"}`, "models"},
		{"empty id", "models.json", `{"providers":{"p":{"apiKey":"k"}},"models":[{"id":"","provider":"p"}]}`, "models[0].id"},
		{"unknown provider", "models.json", `{"providers":{},"models":[{"id":"a","provider":"p"}]}`, "models[0].provider"},
		{"duplicate id", "models.json", `{"providers":{"p":{"apiKey":"k"}},"models":[{"id":"a","provider":"p"},{"id":"a","provider":"p"}]}`, "models[1].id"},
		{"bad strategy", "strategies.json", `{"strategy":"magic","commands":{}}`, "strategy"},
		{"unknown command", "strategies.json", `{"strategy":"cost","commands":{"/nope":{"enabled":true,"models":[]}}}`, "commands./nope"},
		{"two models on one command", "strategies.json", `{"strategy":"cost","commands":{"/fast":{"enabled":true,"models":["a","b"]}}}`, "commands./fast.models"},
		{"trailing data", "strategies.json", `{"strategy":"cost","commands":{}} {}`, "$"},
	}
	for _, tc := range cases {
		t.Run("CFG-5 AC2 "+tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, tc.file, tc.content, 0o600)
			res := load(t, dir)
			if len(res.Problems) == 0 {
				t.Fatal("no problem reported")
			}
			p := res.Problems[0]
			if p.File != tc.file || p.Path != tc.path || p.Reason == "" {
				t.Fatalf("got %+v", p)
			}
			if tc.file == "models.json" && res.Config.Models != nil {
				t.Error("invalid file must not become active")
			}
		})
	}
}

func TestCFG6NeverWrites(t *testing.T) {
	t.Run("CFG-6 AC3 TEST-4 never writes config files", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "models.json", goodModels, 0o600)
		write(t, dir, "strategies.json", goodStrat, 0o600)
		before := map[string][]byte{}
		for _, n := range []string{"models.json", "strategies.json"} {
			before[n], _ = os.ReadFile(filepath.Join(dir, n))
		}
		res := load(t, dir)
		_ = res.Config.Summary()
		for n, b := range before {
			after, _ := os.ReadFile(filepath.Join(dir, n))
			if !bytes.Equal(b, after) {
				t.Errorf("CFG-6 AC3 TEST-4 %s changed", n)
			}
		}
		// invalid content must not be rewritten either
		write(t, dir, "models.json", `{bad`, 0o600)
		load(t, dir)
		if b, _ := os.ReadFile(filepath.Join(dir, "models.json")); string(b) != `{bad` {
			t.Error("CFG-6 invalid models.json was modified")
		}
	})
}

func TestCFG7SummaryHasNoKeys(t *testing.T) {
	t.Run("CFG-7 AC4 summary has no API key", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "models.json", goodModels, 0o600)
		write(t, dir, "strategies.json", goodStrat, 0o600)
		res := load(t, dir)
		s := res.Config.Summary()
		for _, want := range []string{"p", "p/x", "capability", "/fast"} {
			if !strings.Contains(s, want) {
				t.Errorf("CFG-7 summary lacks %q:\n%s", want, s)
			}
		}
		if strings.Contains(s, "SECRET") || strings.Contains(s, "sk-") {
			t.Errorf("CFG-7 AC4 key leaked:\n%s", s)
		}
		all := strings.Join(res.Warnings, "\n")
		if strings.Contains(all, "SECRET") {
			t.Error("CFG-7 key in warnings")
		}
	})
}

func TestCFG8Permissions(t *testing.T) {
	t.Run("CFG-8 warns on broad permissions", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "models.json", goodModels, 0o644)
		if err := os.Chmod(filepath.Join(dir, "models.json"), 0o644); err != nil {
			t.Fatal(err)
		}
		res := load(t, dir)
		if !strings.Contains(strings.Join(res.Warnings, "\n"), "chmod 600") {
			t.Errorf("CFG-8 no permission warning: %v", res.Warnings)
		}
		dir2 := t.TempDir()
		write(t, dir2, "models.json", goodModels, 0o600)
		res = load(t, dir2)
		if strings.Contains(strings.Join(res.Warnings, "\n"), "chmod") {
			t.Error("CFG-8 warned on owner-only file")
		}
	})
}

func TestCFG9MissingMetadata(t *testing.T) {
	t.Run("CFG-9 AC5 model without metadata loads", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "models.json", goodModels, 0o600)
		res := load(t, dir)
		if len(res.Problems) != 0 || res.Config.Models == nil || len(res.Config.Models.Models) != 1 {
			t.Fatalf("CFG-9 AC5 model without metadata rejected: %+v", res.Problems)
		}
	})
}

func TestCFG10UnknownModelMakesCommandUnusable(t *testing.T) {
	t.Run("CFG-10 AC6 unknown model makes command unusable", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "models.json", goodModels, 0o600)
		write(t, dir, "strategies.json",
			`{"strategy":"cost","commands":{"/fast":{"enabled":true,"models":["p/x"]},"/code":{"enabled":true,"models":["ghost/y"]}}}`, 0o600)
		res := load(t, dir)
		if _, bad := res.Config.Unusable["/code"]; !bad {
			t.Fatal("CFG-10 AC6 /code should be unusable")
		}
		if _, bad := res.Config.Unusable["/fast"]; bad {
			t.Fatal("/fast should be usable")
		}
		if !strings.Contains(strings.Join(res.Warnings, "\n"), "/code is unusable") {
			t.Error("no warning")
		}
		if !strings.Contains(res.Config.Summary(), "unusable") {
			t.Error("summary should flag it")
		}
	})
}

func TestCFG5Unreadable(t *testing.T) {
	t.Run("CFG-5 unreadable file is a problem, not a crash", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "models.json"), 0o700); err != nil {
			t.Fatal(err)
		}
		res := load(t, dir)
		if len(res.Problems) != 1 || res.Problems[0].File != "models.json" {
			t.Fatalf("got %+v", res.Problems)
		}
	})
}

func TestAssignedCommands(t *testing.T) { // supports TUI-9
	c := configuration.Config{Strategies: &configuration.StrategiesFile{Strategy: "cost", Commands: map[string]configuration.CommandAssignment{
		"/fast":     {Enabled: true, Models: []string{"a", "b"}},
		"/code":     {Enabled: false, Models: []string{"a"}},
		"/reason":   {Enabled: true, Models: []string{"a"}},
		"/creative": {Enabled: true, Models: []string{"b"}},
	}}}
	got := c.AssignedCommands("a")
	if len(got) != 2 || got[0] != "/fast" || got[1] != "/reason" {
		t.Fatalf("got %v", got)
	}
	if (configuration.Config{}).AssignedCommands("a") != nil {
		t.Fatal("no strategies: want none")
	}
}
