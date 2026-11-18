package configuration_test

import (
	"strings"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
)

// provModels builds a models.json with one provider "p" and one model.
func provModels(provider, model string) string {
	return `{"providers":{"p":` + provider + `},"models":[{"id":"p/x","provider":"p"` + model + `}]}`
}

// problemFor loads models.json and returns the reasons reported for path.
func problemsAt(t *testing.T, models, path string) []string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "models.json", models, 0o600)
	var out []string
	for _, p := range load(t, dir).Problems {
		if p.File == "models.json" && p.Path == path {
			out = append(out, p.Reason)
		}
	}
	return out
}

func TestPROV1TypeRequired(t *testing.T) {
	t.Run("PROV-1 missing type is a problem naming the path", func(t *testing.T) {
		got := problemsAt(t, provModels(`{"apiKey":"x"}`, ""), "providers.p.type")
		if len(got) != 1 || got[0] != "must be one of anthropic, openai, ollama" {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("PROV-1 unknown type is a problem", func(t *testing.T) {
		if got := problemsAt(t, provModels(`{"type":"gemini"}`, ""), "providers.p.type"); len(got) != 1 {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("PROV-1 the three types are valid", func(t *testing.T) {
		for _, ty := range []string{"anthropic", "openai", "ollama"} {
			dir := t.TempDir()
			write(t, dir, "models.json", provModels(`{"type":"`+ty+`"}`, ""), 0o600)
			if res := load(t, dir); len(res.Problems) != 0 || res.Config.Models == nil {
				t.Errorf("%s: %v", ty, res.Problems)
			}
		}
	})
}

func TestPROV2DefaultBaseURL(t *testing.T) {
	for ty, want := range map[string]string{
		"anthropic": "https://api.anthropic.com",
		"openai":    "https://api.openai.com/v1",
		"ollama":    "http://localhost:11434",
	} {
		t.Run("PROV-2 default for "+ty, func(t *testing.T) {
			if got := (configuration.ProviderEntry{Type: ty}).EffectiveBaseURL(); got != want {
				t.Fatalf("got %s", got)
			}
		})
	}
	t.Run("PROV-2 a set baseUrl wins", func(t *testing.T) {
		got := configuration.ProviderEntry{Type: "openai", BaseURL: "http://h:1/v1"}.EffectiveBaseURL()
		if got != "http://h:1/v1" {
			t.Fatalf("got %s", got)
		}
	})
}

func TestPROV3BaseURLValidation(t *testing.T) {
	for _, bad := range []string{"ftp://h", "https://h/v1?x=1", "https://h/v1#f", "/v1", "h:80", "https://", "https://h/v1?"} {
		t.Run("PROV-3 rejects "+bad, func(t *testing.T) {
			got := problemsAt(t, provModels(`{"type":"openai","baseUrl":"`+bad+`"}`, ""), "providers.p.baseUrl")
			if len(got) != 1 {
				t.Fatalf("got %v", got)
			}
		})
	}
	t.Run("PROV-3 trailing slash is ignored", func(t *testing.T) {
		got := configuration.ProviderEntry{Type: "openai", BaseURL: "https://h/v1/"}.EffectiveBaseURL()
		if got != "https://h/v1" {
			t.Fatalf("got %s", got)
		}
		if p := problemsAt(t, provModels(`{"type":"openai","baseUrl":"https://h/v1/"}`, ""), "providers.p.baseUrl"); len(p) != 0 {
			t.Fatalf("got %v", p)
		}
	})
}

func TestPROV4APIKeyOptional(t *testing.T) {
	t.Run("PROV-4 no apiKey is valid for every type", func(t *testing.T) {
		for _, ty := range []string{"anthropic", "openai", "ollama"} {
			dir := t.TempDir()
			write(t, dir, "models.json", provModels(`{"type":"`+ty+`"}`, ""), 0o600)
			if res := load(t, dir); len(res.Problems) != 0 {
				t.Errorf("%s: %v", ty, res.Problems)
			}
		}
	})
}

func warningsOf(t *testing.T, models string) []string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "models.json", models, 0o600)
	var out []string
	for _, w := range load(t, dir).Warnings {
		if strings.Contains(w, "provider \"p\"") {
			out = append(out, w)
		}
	}
	return out
}

func TestPROV5MissingKeyWarning(t *testing.T) {
	t.Run("PROV-5 anthropic without apiKey warns and still loads", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "models.json", provModels(`{"type":"anthropic"}`, ""), 0o600)
		res := load(t, dir)
		if res.Config.Models == nil || len(res.Config.Models.Models) != 1 {
			t.Fatal("model not loaded")
		}
		if got := warningsOf(t, provModels(`{"type":"anthropic"}`, "")); len(got) != 1 || !strings.Contains(got[0], "apiKey") {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("PROV-5 openai with default baseUrl and no key warns", func(t *testing.T) {
		if got := warningsOf(t, provModels(`{"type":"openai"}`, "")); len(got) != 1 {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("PROV-5 openai with a custom baseUrl and no key does not warn", func(t *testing.T) {
		if got := warningsOf(t, provModels(`{"type":"openai","baseUrl":"http://localhost:1234/v1"}`, "")); len(got) != 0 {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("PROV-5 ollama without key does not warn", func(t *testing.T) {
		if got := warningsOf(t, provModels(`{"type":"ollama"}`, "")); len(got) != 0 {
			t.Fatalf("got %v", got)
		}
	})
}

func TestPROV6PlainHTTPWarning(t *testing.T) {
	t.Run("PROV-6 key over http to a remote host warns without the key", func(t *testing.T) {
		got := warningsOf(t, provModels(`{"type":"openai","baseUrl":"http://10.0.0.5:8000/v1","apiKey":"sk-secret"}`, ""))
		if len(got) != 1 || !strings.Contains(got[0], "unencrypted") || strings.Contains(got[0], "sk-secret") {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("PROV-6 loopback hosts do not warn", func(t *testing.T) {
		for _, h := range []string{"localhost", "127.0.0.1", "127.9.9.9", "[::1]"} {
			got := warningsOf(t, provModels(`{"type":"openai","baseUrl":"http://`+h+`:8000/v1","apiKey":"sk-secret"}`, ""))
			if len(got) != 0 {
				t.Errorf("%s: %v", h, got)
			}
		}
	})
	t.Run("PROV-6 https does not warn", func(t *testing.T) {
		got := warningsOf(t, provModels(`{"type":"openai","baseUrl":"https://h/v1","apiKey":"sk-secret"}`, ""))
		if len(got) != 0 {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("PROV-6 no key over http does not warn", func(t *testing.T) {
		got := warningsOf(t, provModels(`{"type":"ollama","baseUrl":"http://10.0.0.5:11434"}`, ""))
		if len(got) != 0 {
			t.Fatalf("got %v", got)
		}
	})
}

func TestPROV7TimeoutSeconds(t *testing.T) {
	for _, tc := range []struct {
		v    string
		good bool
	}{{"1", true}, {"3600", true}, {"0", false}, {"3601", false}, {"-5", false}} {
		t.Run("PROV-7 timeoutSeconds "+tc.v, func(t *testing.T) {
			got := problemsAt(t, provModels(`{"type":"ollama","timeoutSeconds":`+tc.v+`}`, ""), "providers.p.timeoutSeconds")
			if (len(got) == 0) != tc.good {
				t.Fatalf("got %v", got)
			}
		})
	}
	t.Run("PROV-7 a non-integer is a problem", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "models.json", provModels(`{"type":"ollama","timeoutSeconds":1.5}`, ""), 0o600)
		if res := load(t, dir); len(res.Problems) == 0 {
			t.Fatal("want a problem")
		}
	})
}

func TestPROV8ModelName(t *testing.T) {
	t.Run("PROV-8 model field is the API name", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "models.json", provModels(`{"type":"openai"}`, `,"model":"anthropic/claude-x"`), 0o600)
		m := load(t, dir).Config.Models.Models[0]
		if m.APIName() != "anthropic/claude-x" {
			t.Fatalf("got %s", m.APIName())
		}
	})
	t.Run("PROV-8 without model the id is used", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "models.json", provModels(`{"type":"openai"}`, ""), 0o600)
		if got := load(t, dir).Config.Models.Models[0].APIName(); got != "p/x" {
			t.Fatalf("got %s", got)
		}
	})
	t.Run("PROV-8 an empty model is a problem", func(t *testing.T) {
		if got := problemsAt(t, provModels(`{"type":"openai"}`, `,"model":""`), "models[0].model"); len(got) != 1 {
			t.Fatalf("got %v", got)
		}
	})
}

func TestPROV9MaxOutputTokens(t *testing.T) {
	for _, tc := range []struct {
		v    string
		good bool
	}{{"1", true}, {"4096", true}, {"0", false}, {"-1", false}} {
		t.Run("PROV-9 maxOutputTokens "+tc.v, func(t *testing.T) {
			got := problemsAt(t, provModels(`{"type":"openai"}`, `,"maxOutputTokens":`+tc.v), "models[0].maxOutputTokens")
			if (len(got) == 0) != tc.good {
				t.Fatalf("got %v", got)
			}
		})
	}
}
