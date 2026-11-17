package strategies

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
)

func f(v float64) *float64 { return &v }

func price(in, out *float64) *configuration.Pricing {
	return &configuration.Pricing{InputPer1M: in, OutputPer1M: out}
}

func sampleEntries() []metadata.Entry {
	return []metadata.Entry{
		{ID: "a/cheap", Pricing: price(f(1), f(2)), Speed: "slow", Capabilities: []string{"code"}},
		{ID: "a/mid", Pricing: price(f(3), f(4)), Speed: "Fast", Capabilities: []string{"reasoning", "creative"}},
		{ID: "a/nopricing", Speed: "weird", Capabilities: []string{"speed"}},
		{ID: "a/nothing"},
	}
}

type fakeReader struct {
	data map[string][]byte
}

func (r fakeReader) Read(_ context.Context, p string) ([]byte, error) {
	if d, ok := r.data[filepath.Base(p)]; ok {
		return d, nil
	}
	return nil, fs.ErrNotExist
}
func (r fakeReader) Mode(context.Context, string) (fs.FileMode, error) { return 0o600, nil }

func assignment(s Suggestion, cmd string) Assignment {
	for _, a := range s.Assignments {
		if a.Command == cmd {
			return a
		}
	}
	return Assignment{}
}

func jsonBlock(t *testing.T, out string) string {
	t.Helper()
	_, rest, ok := strings.Cut(out, "```json\n")
	if !ok {
		t.Fatalf("no json block in:\n%s", out)
	}
	block, _, ok := strings.Cut(rest, "\n```")
	if !ok {
		t.Fatal("unterminated json block")
	}
	return block
}

func TestSTRAT1_ReadsMetadataAndShowsJSON(t *testing.T) {
	t.Run("STRAT-1 shows a strategies.json shaped block", func(t *testing.T) {
		meta, _ := json.Marshal(metadata.File{Models: sampleEntries()})
		out, err := Service{Reader: fakeReader{data: map[string][]byte{"models-metadata.json": meta}}}.Run(context.Background(), "/h", "cost")
		if err != nil {
			t.Fatal(err)
		}
		var sf configuration.StrategiesFile
		if errs := decodeBlock(t, jsonBlock(t, out), &sf); len(errs) > 0 {
			t.Fatalf("block not valid: %v", errs)
		}
		if sf.Strategy != "cost" || sf.Commands["/fast"].Models[0] != "a/cheap" {
			t.Errorf("unexpected content: %+v", sf)
		}
	})
	t.Run("STRAT-1 missing metadata file gives guidance", func(t *testing.T) {
		out, err := Service{Reader: fakeReader{}}.Run(context.Background(), "/h", "cost")
		if err != nil || !strings.Contains(out, "/refresh") || strings.Contains(out, "```json") {
			t.Errorf("out=%q err=%v", out, err)
		}
	})
	t.Run("STRAT-1 malformed metadata file gives guidance", func(t *testing.T) {
		out, err := Service{Reader: fakeReader{data: map[string][]byte{"models-metadata.json": []byte("{")}}}.Run(context.Background(), "/h", "cost")
		if err != nil || !strings.Contains(out, "not valid") {
			t.Errorf("out=%q err=%v", out, err)
		}
	})
}

func decodeBlock(t *testing.T, block string, sf *configuration.StrategiesFile) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(block))
	dec.DisallowUnknownFields()
	if err := dec.Decode(sf); err != nil {
		return []string{err.Error()}
	}
	var out []string
	for _, e := range sf.Validate() {
		out = append(out, e.Path+": "+e.Reason)
	}
	return out
}

func TestSTRAT1_RejectsInvalidMetadataContent(t *testing.T) {
	for name, body := range map[string]string{
		"negative price": `{"models":[{"id":"x","pricing":{"inputPer1M":-1,"outputPer1M":1}}]}`,
		"bad source":     `{"models":[{"id":"x","speed":"fast","sources":{"speed":"guess"}}]}`,
		"empty id":       `{"models":[{"id":"","speed":"fast"}]}`,
	} {
		t.Run("STRAT-1 invalid metadata: "+name, func(t *testing.T) {
			out, err := Service{Reader: fakeReader{data: map[string][]byte{"models-metadata.json": []byte(body)}}}.Run(context.Background(), "/h", "cost")
			if err != nil || !strings.Contains(out, "not valid") || strings.Contains(out, "```json") {
				t.Errorf("out=%q err=%v", out, err)
			}
		})
	}
}

func TestSTRAT2_Usage(t *testing.T) {
	for _, arg := range []string{"", "  ", "cheap", "cost speed"} {
		t.Run("STRAT-2 usage for "+arg, func(t *testing.T) {
			out, err := Service{Reader: fakeReader{}}.Run(context.Background(), "/h", arg)
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range Names {
				if !strings.Contains(out, n) {
					t.Errorf("usage misses %s", n)
				}
			}
			if strings.Contains(out, "```json") {
				t.Error("must not guess a strategy")
			}
		})
	}
	t.Run("STRAT-2 valid argument is case-insensitive", func(t *testing.T) {
		if s, ok := ParseArg(" Speed "); !ok || s != "speed" {
			t.Errorf("got %q %v", s, ok)
		}
	})
}

func TestSTRAT3_Cost(t *testing.T) {
	t.Run("STRAT-3 lowest cost among usable pricing", func(t *testing.T) {
		s, _ := Suggest("cost", sampleEntries())
		if got := assignment(s, "/code").Model; got != "a/cheap" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("STRAT-3 no pricing means no cost claim", func(t *testing.T) {
		s, _ := Suggest("cost", []metadata.Entry{
			{ID: "x", Capabilities: []string{"code"}},
			{ID: "y", Pricing: price(f(1), nil)}, // only one price: not usable
		})
		for _, a := range s.Assignments {
			if a.Model != "" || strings.Contains(strings.ToLower(a.Reason), "lowest") {
				t.Errorf("%s: %+v", a.Command, a)
			}
		}
	})
	t.Run("STRAT-3 free model is usable", func(t *testing.T) {
		s, _ := Suggest("cost", []metadata.Entry{{ID: "free", Pricing: price(f(0), f(0))}, {ID: "p", Pricing: price(f(1), f(1))}})
		if got := assignment(s, "/fast").Model; got != "free" {
			t.Errorf("got %q", got)
		}
	})
}

func TestSTRAT4_And13_Speed(t *testing.T) {
	t.Run("STRAT-4 picks the fastest known speed", func(t *testing.T) {
		s, _ := Suggest("speed", sampleEntries())
		if got := assignment(s, "/fast").Model; got != "a/mid" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("STRAT-4 no speed data means no ranking", func(t *testing.T) {
		s, _ := Suggest("speed", []metadata.Entry{{ID: "x", Capabilities: []string{"code"}}, {ID: "y", Pricing: price(f(1), f(1))}})
		for _, a := range s.Assignments {
			if a.Model != "" || !strings.Contains(a.Reason, "no ranking") {
				t.Errorf("%+v", a)
			}
		}
	})
	t.Run("STRAT-13 unknown speed values are never ranked", func(t *testing.T) {
		s, _ := Suggest("speed", []metadata.Entry{{ID: "x", Speed: "blazing"}, {ID: "y", Speed: "slow"}})
		if got := assignment(s, "/fast").Model; got != "y" {
			t.Errorf("got %q", got)
		}
		s, _ = Suggest("speed", []metadata.Entry{{ID: "x", Speed: "blazing"}})
		if got := assignment(s, "/fast").Model; got != "" {
			t.Errorf("unknown speed was ranked: %q", got)
		}
	})
	t.Run("STRAT-13 order is fast > medium > slow", func(t *testing.T) {
		s, _ := Suggest("speed", []metadata.Entry{{ID: "s", Speed: "slow"}, {ID: "m", Speed: "medium"}})
		if got := assignment(s, "/fast").Model; got != "m" {
			t.Errorf("got %q", got)
		}
	})
}

func TestSTRAT5_And12_Capability(t *testing.T) {
	t.Run("STRAT-5 best match per routing command", func(t *testing.T) {
		s, _ := Suggest("capability", sampleEntries())
		want := map[string]string{"/fast": "a/nopricing", "/code": "a/cheap", "/reason": "a/mid", "/creative": "a/mid"}
		for cmd, m := range want {
			if got := assignment(s, cmd).Model; got != m {
				t.Errorf("%s: got %q want %q", cmd, got, m)
			}
		}
	})
	t.Run("STRAT-12 derives from capability tags", func(t *testing.T) {
		s, _ := Suggest("capability", []metadata.Entry{{ID: "x", Capabilities: []string{"CODE"}}})
		if assignment(s, "/code").Model != "x" || assignment(s, "/reason").Model != "" {
			t.Errorf("%+v", s.Assignments)
		}
	})
}

func TestSTRAT6_ReasonsOutsideJSON(t *testing.T) {
	t.Run("STRAT-6 reasons are outside the block and the block stays valid", func(t *testing.T) {
		s, _ := Suggest("cost", sampleEntries())
		out, err := s.Render("/h")
		if err != nil {
			t.Fatal(err)
		}
		block := jsonBlock(t, out)
		if strings.Contains(block, "Lowest estimated cost") {
			t.Error("reason inside the JSON block")
		}
		if !strings.Contains(out, "Lowest estimated cost") {
			t.Error("reason missing")
		}
		var sf configuration.StrategiesFile
		if errs := decodeBlock(t, block, &sf); len(errs) > 0 {
			t.Errorf("block invalid: %v", errs)
		}
	})
}

func TestSTRAT7_Unassigned(t *testing.T) {
	t.Run("STRAT-7 command without a fitting model is empty and flagged", func(t *testing.T) {
		s, _ := Suggest("capability", []metadata.Entry{{ID: "x", Capabilities: []string{"code"}}})
		out, _ := s.Render("/h")
		var sf configuration.StrategiesFile
		if errs := decodeBlock(t, jsonBlock(t, out), &sf); len(errs) > 0 {
			t.Fatal(errs)
		}
		ca := sf.Commands["/reason"]
		if len(ca.Models) != 0 || ca.Enabled {
			t.Errorf("%+v", ca)
		}
		if !strings.Contains(out, "- /reason: unassigned.") {
			t.Errorf("not flagged:\n%s", out)
		}
	})
}

func TestSTRAT8_ModelReuseAndD004(t *testing.T) {
	t.Run("STRAT-8 a model is reused across commands, one model per command", func(t *testing.T) {
		s, _ := Suggest("cost", sampleEntries())
		f := s.File()
		for cmd, ca := range f.Commands {
			if len(ca.Models) != 1 || ca.Models[0] != "a/cheap" {
				t.Errorf("%s: %+v", cmd, ca)
			}
		}
		if errs := f.Validate(); len(errs) > 0 {
			t.Error(errs)
		}
	})
	t.Run("STRAT-8 models without metadata are never suggested", func(t *testing.T) {
		s, _ := Suggest("capability", []metadata.Entry{{ID: "nothing"}})
		for _, a := range s.Assignments {
			if a.Model != "" {
				t.Errorf("%+v", a)
			}
		}
	})
}

func TestSTRAT9_NoWrites(t *testing.T) {
	t.Run("STRAT-9 /suggest writes no file", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("AGENT_FACTORY_HOME", dir)
		meta, _ := json.Marshal(metadata.File{Models: sampleEntries()})
		if err := os.WriteFile(filepath.Join(dir, "models-metadata.json"), meta, 0o600); err != nil {
			t.Fatal(err)
		}
		before := snapshot(t, dir)
		for _, arg := range []string{"cost", "speed", "capability", "", "bad"} {
			if _, err := (Service{Reader: osReader{}}).Run(context.Background(), dir, arg); err != nil {
				t.Fatal(err)
			}
		}
		after := snapshot(t, dir)
		if len(before) != len(after) {
			t.Fatalf("files changed: %v -> %v", before, after)
		}
		for k, v := range before {
			if after[k] != v {
				t.Errorf("%s changed", k)
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "strategies.json")); !errors.Is(err, fs.ErrNotExist) {
			t.Error("strategies.json must not be created")
		}
	})
}

type osReader struct{}

func (osReader) Read(_ context.Context, p string) ([]byte, error) { return os.ReadFile(p) }
func (osReader) Mode(_ context.Context, p string) (fs.FileMode, error) {
	st, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	return st.Mode(), nil
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		out[e.Name()] = string(b)
	}
	return out
}

func TestSTRAT10_PasteInstruction(t *testing.T) {
	t.Run("STRAT-10 ends with paste path then /refresh", func(t *testing.T) {
		s, _ := Suggest("speed", sampleEntries())
		out, _ := s.Render("/home/u/.agent-factory")
		want := "Paste into /home/u/.agent-factory/strategies.json, then run /refresh."
		if !strings.HasSuffix(strings.TrimSpace(out), want) {
			t.Errorf("tail:\n%s", out)
		}
	})
}

func TestSTRAT11_SchemaValid(t *testing.T) {
	for _, strat := range Names {
		t.Run("STRAT-11 block passes the schema for "+strat, func(t *testing.T) {
			s, _ := Suggest(strat, sampleEntries())
			out, err := s.Render("/h")
			if err != nil {
				t.Fatal(err)
			}
			var sf configuration.StrategiesFile
			if errs := decodeBlock(t, jsonBlock(t, out), &sf); len(errs) > 0 {
				t.Errorf("%v", errs)
			}
			if len(sf.Commands) != 4 {
				t.Errorf("want 4 commands, got %d", len(sf.Commands))
			}
		})
	}
}
