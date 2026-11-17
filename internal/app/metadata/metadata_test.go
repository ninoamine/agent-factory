package metadata_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
	"github.com/ninoamine/agent-factory/internal/infra/storage"
)

const key = "sk-SECRET-123"

type fakeFetcher struct {
	data map[string]metadata.APIMetadata
	errs map[string]error
}

func (f fakeFetcher) FetchMetadata(_ context.Context, _, id string) (metadata.APIMetadata, error) {
	if err := f.errs[id]; err != nil {
		return metadata.APIMetadata{}, err
	}
	d, ok := f.data[id]
	if !ok {
		return metadata.APIMetadata{}, metadata.ErrUnsupported
	}
	return d, nil
}

func cfg(dir string, models ...configuration.ModelEntry) configuration.Config {
	return configuration.Config{
		Dir: dir,
		Models: &configuration.ModelsFile{
			Providers: map[string]configuration.ProviderEntry{"p": {APIKey: key}},
			Models:    models,
		},
	}
}

func svc(f metadata.Fetcher) metadata.Service {
	return metadata.Service{Reader: storage.Files{}, Writer: storage.Writer{}, Fetcher: f}
}

func entry(t *testing.T, r metadata.Result, id string) metadata.Entry {
	t.Helper()
	for _, e := range r.File.Models {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("model %s not in output", id)
	return metadata.Entry{}
}

func TestMETA1UserMetadataOptional(t *testing.T) {
	t.Run("META-1 user metadata is optional per field", func(t *testing.T) {
		c := cfg(t.TempDir(),
			configuration.ModelEntry{ID: "p/a", Provider: "p", Metadata: &configuration.UserMetadata{Speed: "fast"}},
			configuration.ModelEntry{ID: "p/b", Provider: "p"})
		r, err := svc(nil).Generate(context.Background(), c)
		if err != nil {
			t.Fatal(err)
		}
		if e := entry(t, r, "p/a"); e.Speed != "fast" || len(e.Capabilities) != 0 {
			t.Fatalf("got %+v", e)
		}
	})
}

func TestMETA2And3Merge(t *testing.T) {
	f := fakeFetcher{data: map[string]metadata.APIMetadata{
		"p/a": {Speed: "slow", Capabilities: []string{"code"}},
	}}
	c := cfg(t.TempDir(), configuration.ModelEntry{ID: "p/a", Provider: "p",
		Metadata: &configuration.UserMetadata{Speed: "fast"}})
	r, err := svc(f).Generate(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	e := entry(t, r, "p/a")
	t.Run("META-2 uses API metadata from the adapter", func(t *testing.T) {
		if len(e.Capabilities) != 1 || e.Capabilities[0] != "code" {
			t.Fatalf("got %+v", e)
		}
	})
	t.Run("META-3 AC1 user value wins per field", func(t *testing.T) {
		if e.Speed != "fast" {
			t.Fatalf("speed = %q", e.Speed)
		}
	})
	t.Run("META-11 writes the source per field", func(t *testing.T) {
		if e.Sources == nil || e.Sources.Speed != metadata.SourceUser || e.Sources.Capabilities != metadata.SourceAPI {
			t.Fatalf("sources = %+v", e.Sources)
		}
	})
}

func TestMETA4And5Unknown(t *testing.T) {
	dir := t.TempDir()
	c := cfg(dir, configuration.ModelEntry{ID: "p/none", Provider: "p"})
	r, err := svc(fakeFetcher{}).Generate(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("META-4 AC2 unknown fields stay absent", func(t *testing.T) {
		e := entry(t, r, "p/none")
		if e.Speed != "" || e.Capabilities != nil || e.Sources != nil {
			t.Fatalf("invented values: %+v", e)
		}
		data, _ := os.ReadFile(filepath.Join(dir, "models-metadata.json"))
		if strings.Contains(string(data), "speed") {
			t.Fatalf("file mentions speed: %s", data)
		}
	})
	t.Run("META-5 model without metadata is flagged with a warning", func(t *testing.T) {
		if len(r.NoMetadata) != 1 || r.NoMetadata[0] != "p/none" {
			t.Fatalf("NoMetadata = %v", r.NoMetadata)
		}
		if len(r.Warnings) == 0 || !strings.Contains(r.Warnings[0], "p/none") {
			t.Fatalf("warnings = %v", r.Warnings)
		}
	})
}

func TestMETA6NoKeys(t *testing.T) {
	t.Run("META-6 AC3 output has no key strings", func(t *testing.T) {
		dir := t.TempDir()
		f := fakeFetcher{errs: map[string]error{"p/a": errors.New("auth failed for " + key)}}
		c := cfg(dir,
			configuration.ModelEntry{ID: "p/a", Provider: "p"},
			configuration.ModelEntry{ID: "p/b", Provider: "p", Metadata: &configuration.UserMetadata{Speed: "fast"}})
		r, err := svc(f).Generate(context.Background(), c)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(dir, "models-metadata.json"))
		all := string(data) + strings.Join(r.Warnings, "\n")
		if strings.Contains(all, key) || strings.Contains(all, "apiKey") {
			t.Fatalf("key leaked: %s", all)
		}
	})
}

func TestMETA7Atomic(t *testing.T) {
	t.Run("META-7 writes atomically, leaves no temp file", func(t *testing.T) {
		dir := t.TempDir()
		c := cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p", Metadata: &configuration.UserMetadata{Speed: "fast"}})
		if _, err := svc(nil).Generate(context.Background(), c); err != nil {
			t.Fatal(err)
		}
		ents, _ := os.ReadDir(dir)
		if len(ents) != 1 || ents[0].Name() != "models-metadata.json" {
			t.Fatalf("dir = %v", ents)
		}
	})
	t.Run("META-7 writer refuses user-owned files", func(t *testing.T) {
		for _, n := range []string{"models.json", "strategies.json"} {
			p := filepath.Join(t.TempDir(), n)
			if err := (storage.Writer{}).WriteAtomic(context.Background(), p, []byte("x")); err == nil {
				t.Fatalf("wrote %s", n)
			}
			if _, err := os.Stat(p); err == nil {
				t.Fatalf("%s exists", n)
			}
		}
	})
}

func TestMETA8FetchFailure(t *testing.T) {
	t.Run("META-8 AC4 failure keeps last known API data, others written", func(t *testing.T) {
		dir := t.TempDir()
		c := cfg(dir,
			configuration.ModelEntry{ID: "p/a", Provider: "p"},
			configuration.ModelEntry{ID: "p/b", Provider: "p"})
		ok := fakeFetcher{data: map[string]metadata.APIMetadata{
			"p/a": {Speed: "fast"}, "p/b": {Speed: "slow"}}}
		if _, err := svc(ok).Generate(context.Background(), c); err != nil {
			t.Fatal(err)
		}
		bad := fakeFetcher{
			data: map[string]metadata.APIMetadata{"p/b": {Speed: "medium"}},
			errs: map[string]error{"p/a": errors.New("boom")},
		}
		r, err := svc(bad).Generate(context.Background(), c)
		if err != nil {
			t.Fatal(err)
		}
		if e := entry(t, r, "p/a"); e.Speed != "fast" {
			t.Fatalf("last known lost: %+v", e)
		}
		if e := entry(t, r, "p/b"); e.Speed != "medium" {
			t.Fatalf("other model not written: %+v", e)
		}
		if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "p/a") {
			t.Fatalf("warnings = %v", r.Warnings)
		}
	})
	t.Run("META-8 failure without last known invents nothing", func(t *testing.T) {
		c := cfg(t.TempDir(), configuration.ModelEntry{ID: "p/a", Provider: "p"})
		f := fakeFetcher{errs: map[string]error{"p/a": errors.New("boom")}}
		r, err := svc(f).Generate(context.Background(), c)
		if err != nil {
			t.Fatal(err)
		}
		if e := entry(t, r, "p/a"); e.HasMetadata() {
			t.Fatalf("invented: %+v", e)
		}
	})
	t.Run("META-8 invalid provider data is treated as a failure", func(t *testing.T) {
		c := cfg(t.TempDir(), configuration.ModelEntry{ID: "p/a", Provider: "p"})
		f := fakeFetcher{data: map[string]metadata.APIMetadata{"p/a": {Capabilities: []string{""}}}}
		r, _ := svc(f).Generate(context.Background(), c)
		if entry(t, r, "p/a").HasMetadata() || len(r.Warnings) == 0 {
			t.Fatalf("got %+v %v", r.File, r.Warnings)
		}
	})
}

func TestMETA9Startup(t *testing.T) {
	dir := t.TempDir()
	c := cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p", Metadata: &configuration.UserMetadata{Speed: "fast"}})
	path := filepath.Join(dir, "models-metadata.json")
	t.Run("META-9 generates when absent", func(t *testing.T) {
		_, gen, err := svc(nil).GenerateIfAbsent(context.Background(), c)
		if err != nil || !gen {
			t.Fatalf("gen=%v err=%v", gen, err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("META-9 does not regenerate when present", func(t *testing.T) {
		if err := os.WriteFile(path, []byte(`{"models":[]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, gen, err := svc(nil).GenerateIfAbsent(context.Background(), c)
		if err != nil || gen {
			t.Fatalf("gen=%v err=%v", gen, err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != `{"models":[]}` {
			t.Fatalf("file changed: %s", data)
		}
	})
}

func TestMETA10UserDataStable(t *testing.T) {
	t.Run("META-10 AC5 regenerating twice keeps user values", func(t *testing.T) {
		dir := t.TempDir()
		c := cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p",
			Metadata: &configuration.UserMetadata{Speed: "fast", Capabilities: []string{"code"}}})
		f := fakeFetcher{data: map[string]metadata.APIMetadata{"p/a": {Speed: "slow", Capabilities: []string{"chat"}}}}
		var out [2][]byte
		for i := range out {
			if _, err := svc(f).Generate(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			out[i], _ = os.ReadFile(filepath.Join(dir, "models-metadata.json"))
		}
		if string(out[0]) != string(out[1]) {
			t.Fatalf("output changed:\n%s\n%s", out[0], out[1])
		}
		if !strings.Contains(string(out[1]), `"fast"`) || !strings.Contains(string(out[1]), `"code"`) {
			t.Fatalf("user values lost: %s", out[1])
		}
	})
	t.Run("META-10 user values are not kept as last known API data", func(t *testing.T) {
		dir := t.TempDir()
		with := cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p", Metadata: &configuration.UserMetadata{Speed: "fast"}})
		if _, err := svc(nil).Generate(context.Background(), with); err != nil {
			t.Fatal(err)
		}
		without := cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p"})
		f := fakeFetcher{errs: map[string]error{"p/a": errors.New("boom")}}
		r, _ := svc(f).Generate(context.Background(), without)
		if entry(t, r, "p/a").Speed != "" {
			t.Fatalf("removed user value resurfaced: %+v", r.File)
		}
	})
}

func f64(v float64) *float64 { return &v }

func TestPricing(t *testing.T) {
	t.Run("META-1 META-3 user pricing wins over API pricing", func(t *testing.T) {
		f := fakeFetcher{data: map[string]metadata.APIMetadata{"p/a": {Pricing: &configuration.Pricing{InputPer1M: f64(3), OutputPer1M: f64(15)}}}}
		c := cfg(t.TempDir(), configuration.ModelEntry{ID: "p/a", Provider: "p",
			Metadata: &configuration.UserMetadata{Pricing: &configuration.Pricing{InputPer1M: f64(1), OutputPer1M: f64(2)}}})
		r, err := svc(f).Generate(context.Background(), c)
		if err != nil {
			t.Fatal(err)
		}
		e := entry(t, r, "p/a")
		if *e.Pricing.InputPer1M != 1 || e.Sources.Pricing != metadata.SourceUser {
			t.Fatalf("got %+v %+v", e.Pricing, e.Sources)
		}
	})
	t.Run("META-3 META-11 API pricing used when user has none", func(t *testing.T) {
		f := fakeFetcher{data: map[string]metadata.APIMetadata{"p/a": {Pricing: &configuration.Pricing{InputPer1M: f64(3)}}}}
		r, _ := svc(f).Generate(context.Background(), cfg(t.TempDir(), configuration.ModelEntry{ID: "p/a", Provider: "p"}))
		e := entry(t, r, "p/a")
		if *e.Pricing.InputPer1M != 3 || e.Pricing.OutputPer1M != nil || e.Sources.Pricing != metadata.SourceAPI {
			t.Fatalf("got %+v", e)
		}
	})
	t.Run("META-4 unknown pricing stays absent", func(t *testing.T) {
		dir := t.TempDir()
		svc(nil).Generate(context.Background(), cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p"}))
		data, _ := os.ReadFile(filepath.Join(dir, "models-metadata.json"))
		if strings.Contains(string(data), "pricing") {
			t.Fatalf("invented: %s", data)
		}
	})
	t.Run("META-8 negative API price is a failure, last known kept", func(t *testing.T) {
		dir := t.TempDir()
		c := cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p"})
		good := fakeFetcher{data: map[string]metadata.APIMetadata{"p/a": {Pricing: &configuration.Pricing{InputPer1M: f64(3)}}}}
		svc(good).Generate(context.Background(), c)
		bad := fakeFetcher{data: map[string]metadata.APIMetadata{"p/a": {Pricing: &configuration.Pricing{InputPer1M: f64(-1)}}}}
		r, _ := svc(bad).Generate(context.Background(), c)
		if e := entry(t, r, "p/a"); e.Pricing == nil || *e.Pricing.InputPer1M != 3 || len(r.Warnings) != 1 {
			t.Fatalf("got %+v %v", e, r.Warnings)
		}
	})
	t.Run("META-10 zero price survives regeneration", func(t *testing.T) {
		dir := t.TempDir()
		c := cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p",
			Metadata: &configuration.UserMetadata{Pricing: &configuration.Pricing{InputPer1M: f64(0)}}})
		svc(nil).Generate(context.Background(), c)
		a, _ := os.ReadFile(filepath.Join(dir, "models-metadata.json"))
		svc(nil).Generate(context.Background(), c)
		b, _ := os.ReadFile(filepath.Join(dir, "models-metadata.json"))
		if string(a) != string(b) || !strings.Contains(string(a), `"inputPer1M": 0`) {
			t.Fatalf("%s\n%s", a, b)
		}
	})
}

func TestEdgeCases(t *testing.T) {
	t.Run("META-8 unsupported adapter keeps last known API data", func(t *testing.T) {
		dir := t.TempDir()
		c := cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p"})
		if _, err := svc(fakeFetcher{data: map[string]metadata.APIMetadata{"p/a": {Speed: "fast"}}}).Generate(context.Background(), c); err != nil {
			t.Fatal(err)
		}
		r, err := svc(fakeFetcher{}).Generate(context.Background(), c) // returns ErrUnsupported
		if err != nil {
			t.Fatal(err)
		}
		if e := entry(t, r, "p/a"); e.Speed != "fast" || e.Sources.Speed != metadata.SourceAPI {
			t.Fatalf("last known dropped: %+v", e)
		}
	})
	t.Run("META-9 unreadable existing file is not overwritten and warns", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "models-metadata.json"), 0o700); err != nil {
			t.Fatal(err)
		}
		c := cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p"})
		r, gen, err := svc(nil).GenerateIfAbsent(context.Background(), c)
		if err != nil || gen || len(r.Warnings) != 1 {
			t.Fatalf("gen=%v err=%v warnings=%v", gen, err, r.Warnings)
		}
	})
	t.Run("META-9 cancelled context returns an error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err := svc(nil).GenerateIfAbsent(ctx, cfg(t.TempDir(), configuration.ModelEntry{ID: "p/a", Provider: "p"}))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("META-7 no valid models.json: nothing is written", func(t *testing.T) {
		dir := t.TempDir()
		r, err := svc(nil).Generate(context.Background(), configuration.Config{Dir: dir})
		if err != nil || len(r.File.Models) != 0 {
			t.Fatalf("r=%+v err=%v", r, err)
		}
		if ents, _ := os.ReadDir(dir); len(ents) != 0 {
			t.Fatalf("wrote files: %v", ents)
		}
	})
	t.Run("META-7 generation never modifies models.json or strategies.json (ARCH-7)", func(t *testing.T) {
		dir := t.TempDir()
		files := map[string]string{
			"models.json":     `{"providers":{"p":{"apiKey":"` + key + `"}},"models":[{"id":"p/a","provider":"p"}]}`,
			"strategies.json": `{"strategy":"cost","commands":{}}`,
		}
		for n, c := range files {
			if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		res, err := configuration.Load(context.Background(), storage.Files{}, dir)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if _, err := svc(nil).Generate(context.Background(), res.Config); err != nil {
				t.Fatal(err)
			}
		}
		for n, c := range files {
			got, _ := os.ReadFile(filepath.Join(dir, n))
			if string(got) != c {
				t.Fatalf("%s changed", n)
			}
		}
	})
}

type blockingFetcher struct{}

func (blockingFetcher) FetchMetadata(ctx context.Context, _, _ string) (metadata.APIMetadata, error) {
	<-ctx.Done()
	return metadata.APIMetadata{}, ctx.Err()
}

func TestReviewFixes(t *testing.T) {
	t.Run("META-8 cancelled context stops generation with an error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		go cancel()
		c := cfg(t.TempDir(), configuration.ModelEntry{ID: "p/a", Provider: "p"}, configuration.ModelEntry{ID: "p/b", Provider: "p"})
		_, err := svc(blockingFetcher{}).Generate(ctx, c)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("META-8 unknown source value in previous file is ignored with a warning", func(t *testing.T) {
		dir := t.TempDir()
		prev := `{"models":[{"id":"p/a","speed":"fast","sources":{"speed":"bogus"}}]}`
		if err := os.WriteFile(filepath.Join(dir, "models-metadata.json"), []byte(prev), 0o600); err != nil {
			t.Fatal(err)
		}
		f := fakeFetcher{errs: map[string]error{"p/a": errors.New("boom")}}
		r, err := svc(f).Generate(context.Background(), cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p"}))
		if err != nil {
			t.Fatal(err)
		}
		if entry(t, r, "p/a").Speed != "" || len(r.Warnings) < 2 {
			t.Fatalf("got %+v %v", r.File, r.Warnings)
		}
	})
	t.Run("META-9 unreadable-file warning has no key text", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "models-metadata.json"), 0o700); err != nil {
			t.Fatal(err)
		}
		c := cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p"})
		c.Models.Providers["p"] = configuration.ProviderEntry{APIKey: filepath.Base(dir)} // key text inside the path
		r, _, _ := svc(nil).GenerateIfAbsent(context.Background(), c)
		if len(r.Warnings) != 1 || strings.Contains(r.Warnings[0], filepath.Base(dir)) {
			t.Fatalf("warnings = %v", r.Warnings)
		}
	})
}

func TestReviewFixes2(t *testing.T) {
	t.Run("META-8 fetch timeout warns and keeps going", func(t *testing.T) {
		s := svc(blockingFetcher{})
		s.FetchTimeout = 10 * time.Millisecond
		c := cfg(t.TempDir(), configuration.ModelEntry{ID: "p/a", Provider: "p"}, configuration.ModelEntry{ID: "p/b", Provider: "p"})
		r, err := s.Generate(context.Background(), c)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.File.Models) != 2 || !strings.Contains(r.Warnings[0], "could not fetch") {
			t.Fatalf("got %+v %v", r.File, r.Warnings)
		}
	})
	t.Run("META-8 invalid provider data is named as such in the warning", func(t *testing.T) {
		f := fakeFetcher{data: map[string]metadata.APIMetadata{"p/a": {Speed: " "}}}
		r, _ := svc(f).Generate(context.Background(), cfg(t.TempDir(), configuration.ModelEntry{ID: "p/a", Provider: "p"}))
		if !strings.Contains(r.Warnings[0], "invalid metadata") {
			t.Fatalf("warnings = %v", r.Warnings)
		}
	})
	t.Run("META-6 previous-file warnings have no key text", func(t *testing.T) {
		dir := t.TempDir()
		bad := `{"models":[{"id":"` + key + `"` // truncated JSON mentioning a key-like id
		os.WriteFile(filepath.Join(dir, "models-metadata.json"), []byte(bad), 0o600)
		c := cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p"})
		r, _ := svc(nil).Generate(context.Background(), c)
		if strings.Contains(strings.Join(r.Warnings, "\n"), key) {
			t.Fatalf("leaked: %v", r.Warnings)
		}
	})
}

func TestREFRESH2_RegeneratorAdapter(t *testing.T) {
	dir := t.TempDir()
	c := cfg(dir, configuration.ModelEntry{ID: "p/a", Provider: "p"}, configuration.ModelEntry{ID: "p/b", Provider: "p"})
	f := fakeFetcher{
		data: map[string]metadata.APIMetadata{"p/a": {Speed: "fast"}},
		errs: map[string]error{"p/b": errors.New("boom")},
	}
	t.Run("REFRESH-2 regenerates the file through the adapter", func(t *testing.T) {
		if _, err := svc(f).Regenerator()(context.Background(), c); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, configuration.MetadataFileName)); err != nil {
			t.Fatalf("models-metadata.json not written: %v", err)
		}
	})
	t.Run("REFRESH-5 a fetch failure is a warning, not an error", func(t *testing.T) {
		warns, err := svc(f).Regenerator()(context.Background(), c)
		if err != nil {
			t.Fatalf("fetch failure must not fail the refresh: %v", err)
		}
		if len(warns) == 0 || !strings.Contains(strings.Join(warns, " "), "p/b") {
			t.Errorf("warnings: %v", warns)
		}
	})
	t.Run("REFRESH-3 a cancelled context returns an error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := svc(f).Regenerator()(ctx, c); err == nil {
			t.Fatal("expected error")
		}
	})
}
