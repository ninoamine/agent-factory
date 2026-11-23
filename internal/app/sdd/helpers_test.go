package sdd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/history"
	"github.com/ninoamine/agent-factory/internal/infra/storage"
)

// ---- fakes (TEST-2: no network, no real keys) ----

// fakeAdapter replies with scripted texts, streaming them, and records prompts.
type fakeAdapter struct {
	mu      sync.Mutex
	replies []string
	prompts []string
	models  []string
	gate    chan struct{} // when set, Stream blocks until closed
	err     error         // when set, Stream fails with it
}

func (a *fakeAdapter) Execute(ctx context.Context, model, prompt string) (execution.Response, error) {
	return a.Stream(ctx, model, prompt, func(string) {})
}

func (a *fakeAdapter) Stream(ctx context.Context, model, prompt string, on func(string)) (execution.Response, error) {
	a.mu.Lock()
	a.prompts = append(a.prompts, prompt)
	a.models = append(a.models, model)
	text := ""
	if len(a.replies) > 0 {
		text, a.replies = a.replies[0], a.replies[1:]
	}
	gate, err := a.gate, a.err
	a.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if err != nil {
		return execution.Response{}, err
	}
	on(text)
	return execution.Response{Text: text}, nil
}

func (a *fakeAdapter) calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.prompts)
}

func (a *fakeAdapter) lastPrompt() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.prompts) == 0 {
		return ""
	}
	return a.prompts[len(a.prompts)-1]
}

type resolver struct{ a execution.Adapter }

func (r resolver) Executor(string) (execution.Adapter, bool) { return r.a, true }

type memStore struct {
	mu   sync.Mutex
	data []byte
}

func (m *memStore) Read(context.Context) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]byte(nil), m.data...), nil
}
func (m *memStore) Append(_ context.Context, d []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = append(m.data, d...)
	return nil
}
func (m *memStore) Rewrite(_ context.Context, d []byte) error { m.data = d; return nil }

type recOut struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (o *recOut) w(s string)      { o.mu.Lock(); o.buf.WriteString(s + "\n"); o.mu.Unlock() }
func (o *recOut) Chunk(t string)  { o.w(t) }
func (o *recOut) Final(t string)  { o.w(t) }
func (o *recOut) Notice(t string) { o.w(t) }
func (o *recOut) Error(t string)  { o.w("ERR " + t) }
func (o *recOut) String() string  { o.mu.Lock(); defer o.mu.Unlock(); return o.buf.String() }

type fakeGit struct {
	diff      string
	untracked []string
	err       error
	calls     []string
}

func (g *fakeGit) Diff(context.Context, string) (string, error) {
	g.calls = append(g.calls, "diff")
	return g.diff, g.err
}
func (g *fakeGit) Untracked(context.Context, string) ([]string, error) {
	g.calls = append(g.calls, "untracked")
	return g.untracked, g.err
}

type fakeEditor struct {
	command string
	got     string
	result  string
	err     error
	called  int
}

func (e *fakeEditor) Edit(_ context.Context, command, content string) (string, error) {
	e.called++
	e.command, e.got = command, content
	return e.result, e.err
}

// ---- environment ----

type env struct {
	t       *testing.T
	dir     string
	svc     *Service
	adapter *fakeAdapter
	out     *recOut
	hist    *memStore
	git     *fakeGit
	editor  *fakeEditor
	vars    map[string]string
	cfg     configuration.Config
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_FACTORY_HOME", t.TempDir())
	e := &env{t: t, dir: dir, adapter: &fakeAdapter{}, out: &recOut{}, hist: &memStore{},
		git: &fakeGit{}, editor: &fakeEditor{}, vars: map[string]string{}}
	e.cfg = configuration.Config{
		Models: &configuration.ModelsFile{
			Providers: map[string]configuration.ProviderEntry{"p": {APIKey: "sk-test-SECRET"}},
			Models: []configuration.ModelEntry{{ID: "m-code", Provider: "p"}, {ID: "m-reason", Provider: "p"},
				{ID: "m-fast", Provider: "p"}},
		},
		Strategies: &configuration.StrategiesFile{Strategy: "cost", Commands: map[string]configuration.CommandAssignment{
			"/code":   {Enabled: true, Models: []string{"m-code"}},
			"/reason": {Enabled: true, Models: []string{"m-reason"}},
			"/fast":   {Enabled: true, Models: []string{"m-fast"}},
		}},
		Unusable: map[string]string{},
	}
	hist := &history.Service{Store: e.hist}
	e.svc = &Service{
		Dir: dir, Files: storage.ProjectFiles{}, Git: e.git, Editor: e.editor,
		Runner: execution.Service{Providers: resolver{e.adapter}, History: hist},
		Config: func() (configuration.Config, func()) { return e.cfg, func() {} },
		Getenv: func(k string) string { return e.vars[k] },
	}
	return e
}

func (e *env) write(rel, content string) {
	e.t.Helper()
	p := filepath.Join(e.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) read(rel string) string {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.dir, filepath.FromSlash(rel)))
	if err != nil {
		e.t.Fatal(err)
	}
	return string(b)
}

func (e *env) has(rel string) bool {
	_, err := os.Lstat(filepath.Join(e.dir, filepath.FromSlash(rel)))
	return err == nil
}

func featureText(id, status string, modules string) string {
	return "---\nid: " + id + "\nstatus: " + status + "\nversion: V1\ndepends_on: []\nmodules: [" + modules + "]\n---\n# " + id + "\n\n- " + id + "-1 first rule\n"
}

// project creates an initialized project with feature AUTH in the given status.
func (e *env) project(status string) {
	e.t.Helper()
	e.write("specs/index.md", "# Spec index\n\n| ID | File | Status | Version | depends_on | Modules |\n|---|---|---|---|---|---|\n| AUTH | features/auth.md | "+status+" | V1 | none | src/auth |\n")
	e.write("specs/features/auth.md", featureText("AUTH", status, "src/auth"))
	e.write("specs/decisions.md", "# Decisions\n\n- D-001 first\n")
	e.write("specs/common/project.md", "# Project\n")
	e.write("AGENTS.md", "# Agents\n")
	e.write("src/auth/login.go", "package auth\n")
}

func block(path, content string) string {
	return "=== FILE: " + path + " ===\n" + content + "=== END FILE ===\n"
}

// run executes a /sdd command with the next scripted reply.
func (e *env) run(args string, replies ...string) (Reply, error) {
	e.t.Helper()
	e.adapter.mu.Lock()
	e.adapter.replies = append(e.adapter.replies, replies...)
	e.adapter.mu.Unlock()
	return e.svc.Run(context.Background(), args, e.out)
}

func (e *env) answer(line string, replies ...string) (Reply, error) {
	e.t.Helper()
	e.adapter.mu.Lock()
	e.adapter.replies = append(e.adapter.replies, replies...)
	e.adapter.mu.Unlock()
	return e.svc.Answer(context.Background(), line, e.out)
}

func joined(r Reply) string { return strings.Join(r.Lines, "\n") }

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// fakeFilesCheck only proves that the Files interface can be implemented by tests.
type fakeFilesCheck struct{ storage.ProjectFiles }
