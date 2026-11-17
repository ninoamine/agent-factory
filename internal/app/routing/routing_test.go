package routing

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/domain"
)

func cfg() configuration.Config {
	return configuration.Config{
		Strategies: &configuration.StrategiesFile{
			Strategy: "cost",
			Commands: map[string]configuration.CommandAssignment{
				"/fast":     {Enabled: true, Models: []string{"m1"}},
				"/code":     {Enabled: true, Models: []string{"m2", "m3"}},
				"/reason":   {Enabled: false, Models: []string{"m1"}},
				"/creative": {Enabled: true},
			},
		},
		Unusable: map[string]string{},
	}
}

func TestROUTE1_FixedCommands(t *testing.T) {
	t.Run("ROUTE-1 exactly four routing commands", func(t *testing.T) {
		want := []string{"/fast", "/code", "/reason", "/creative"}
		if len(domain.RoutingCommands) != 4 {
			t.Fatal("expected 4 commands")
		}
		for i, c := range domain.RoutingCommands {
			if string(c) != want[i] {
				t.Errorf("got %s want %s", c, want[i])
			}
		}
	})
	t.Run("ROUTE-1 custom command refused", func(t *testing.T) {
		if d := Route(cfg(), "/mine hi"); d.Kind != Refused {
			t.Fatal("custom command must be refused")
		}
	})
}

func TestROUTE2_UsableCommandRoutes(t *testing.T) {
	d := Route(cfg(), "/fast hello")
	if d.Kind != Routed || d.Model != "m1" {
		t.Fatalf("ROUTE-2 got %+v", d)
	}
}

func TestROUTE3_NoInference(t *testing.T) {
	for _, in := range []string{"fix this bug", "", "   ", "hello /code"} {
		if d := Route(cfg(), in); d.Kind != Refused || d.Model != "" {
			t.Errorf("ROUTE-3 %q: got %+v", in, d)
		}
	}
}

func TestROUTE4_RefusalMessages(t *testing.T) {
	c := cfg()
	cases := map[string]struct{ in, want string }{
		"missing":    {"hello", "must start with a routing command"},
		"unknown":    {"/nope hi", "unknown command"},
		"disabled":   {"/reason hi", "disabled"},
		"unassigned": {"/creative hi", "no model assigned"},
	}
	seen := map[string]bool{}
	for name, tc := range cases {
		t.Run("ROUTE-4 "+name, func(t *testing.T) {
			d := Route(c, tc.in)
			if d.Kind != Refused || !strings.Contains(d.Message, tc.want) {
				t.Fatalf("got %+v", d)
			}
			if !strings.Contains(d.Message, "Available commands: /fast, /code.") {
				t.Errorf("missing available list: %s", d.Message)
			}
			if seen[d.Message] {
				t.Error("messages must be distinct")
			}
			seen[d.Message] = true
		})
	}
	t.Run("ROUTE-4 unusable command refused", func(t *testing.T) {
		c := cfg()
		c.Unusable["/fast"] = `model "m1" is not in models.json`
		d := Route(c, "/fast hi")
		if d.Kind != Refused || !strings.Contains(d.Message, "unusable") {
			t.Fatalf("got %+v", d)
		}
	})
	t.Run("ROUTE-4 no strategies loaded", func(t *testing.T) {
		d := Route(configuration.Config{}, "/fast hi")
		if d.Kind != Refused || !strings.Contains(d.Message, "none") {
			t.Fatalf("got %+v", d)
		}
	})
}

func TestROUTE5_StripsCommand(t *testing.T) {
	if d := Route(cfg(), "/code fix this"); d.Prompt != "fix this" {
		t.Fatalf("ROUTE-5 got %q", d.Prompt)
	}
	if d := Route(cfg(), "  /code\n  multi\nline "); d.Prompt != "multi\nline" {
		t.Fatalf("ROUTE-5 got %q", d.Prompt)
	}
}

func TestROUTE6_FirstModel(t *testing.T) {
	if d := Route(cfg(), "/code x"); d.Model != "m2" {
		t.Fatalf("ROUTE-6 got %q", d.Model)
	}
}

func TestROUTE7_ToolCommands(t *testing.T) {
	for _, tc := range domain.ToolCommands {
		for _, in := range []string{tc, tc + " extra"} {
			d := Route(configuration.Config{}, in)
			if d.Kind != ToolCommand || d.Tool != tc || d.Model != "" {
				t.Errorf("ROUTE-7 %q: got %+v", in, d)
			}
		}
	}
}

func TestROUTE8_NoProviderImports(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pkgs {
		for fn, f := range p.Files {
			for _, imp := range f.Imports {
				if strings.Contains(imp.Path.Value, "infra") || strings.Contains(imp.Path.Value, "provider") {
					t.Errorf("ROUTE-8 %s imports %s", fn, imp.Path.Value)
				}
			}
		}
	}
	if d := Route(cfg(), "/fast hi"); d.Model == "" {
		t.Error("ROUTE-8 must return a model reference")
	}
}

func TestROUTE9_Independent(t *testing.T) {
	c := cfg()
	a := Route(c, "/fast one")
	b := Route(c, "two")
	if a.Kind != Routed || b.Kind != Refused {
		t.Fatal("ROUTE-9 a prior request must not affect the next")
	}
}

func TestROUTE10_EmptyPrompt(t *testing.T) {
	for _, in := range []string{"/code", "/code   ", "/code\n"} {
		d := Route(cfg(), in)
		if d.Kind != Refused || !strings.Contains(d.Message, "needs a prompt") {
			t.Errorf("ROUTE-10 %q: got %+v", in, d)
		}
	}
}

func TestROUTE11_CaseSensitive(t *testing.T) {
	for _, in := range []string{"/Code hi", "/FAST hi", "/Refresh"} {
		if d := Route(cfg(), in); d.Kind != Refused {
			t.Errorf("ROUTE-11 %q: got %+v", in, d)
		}
	}
}
