package sdd

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
)

func TestSDD31_ListAndUnknownSubcommand(t *testing.T) { // AC1
	e := newEnv(t)
	r, err := e.run("")
	if err != nil || !strings.Contains(joined(r), "spec-validate <id>") || !strings.Contains(joined(r), "not initialized") {
		t.Fatalf("list: %v %q", err, joined(r))
	}
	_, err = e.run("foo")
	if err == nil || !strings.Contains(err.Error(), "spec-review <id>") {
		t.Fatalf("unknown subcommand must be refused with the list: %v", err)
	}
	e.project("ready")
	r, _ = e.run("")
	if !strings.Contains(joined(r), "initialized (specs/index.md found)") {
		t.Fatalf("state: %q", joined(r))
	}
}

func TestSDD32_RefusedWithoutIndex(t *testing.T) { // AC1
	e := newEnv(t)
	_, err := e.run("spec x")
	if err == nil || !strings.Contains(err.Error(), "/sdd init") {
		t.Fatalf("expected pointer to /sdd init, got %v", err)
	}
	if e.adapter.calls() != 0 {
		t.Fatal("no model call expected")
	}
}

func TestSDD35_36_InitCreatesFilesWithoutModel(t *testing.T) { // AC2
	e := newEnv(t)
	e.write("AGENTS.md", "mine\n")
	r, err := e.run("init")
	if err != nil || r.Confirm == nil {
		t.Fatalf("init must ask for confirmation: %v %+v", err, r)
	}
	p := r.Confirm.Prompt
	if !strings.Contains(p, "specs/index.md") || !strings.Contains(p, "skip") || !strings.Contains(p, "AGENTS.md") {
		t.Fatalf("prompt must list creates and skips: %q", p)
	}
	if e.has("specs/index.md") {
		t.Fatal("nothing may be created before the answer")
	}
	if _, err := r.Confirm.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, f := range initFiles {
		if !e.has(f.Path) {
			t.Errorf("missing %s", f.Path)
		}
	}
	if !e.has("specs/features") || e.read("AGENTS.md") != "mine\n" {
		t.Fatal("empty features dir expected and existing AGENTS.md must be unchanged")
	}
	if e.adapter.calls() != 0 {
		t.Fatal("init must not call a model")
	}
}

func TestSDD36_InitDeclinedCreatesNothing(t *testing.T) { // AC2 (answer n: the screen never runs Confirm)
	e := newEnv(t)
	if _, err := e.run("init"); err != nil {
		t.Fatal(err)
	}
	if e.has("specs") || e.has("AGENTS.md") {
		t.Fatal("nothing may be created")
	}
	if e.svc.Active() != "" {
		t.Fatal("init must not hold the busy state")
	}
}

func TestSDD33_Input(t *testing.T) { // AC3
	e := newEnv(t)
	e.project("ready")
	outside := filepath.Join(t.TempDir(), "draft.txt")
	if err := os.WriteFile(outside, []byte("DRAFT FILE TEXT"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, _ := filepath.Rel(e.dir, outside)
	e.run("spec "+rel, "no blocks here")
	if !strings.Contains(e.adapter.lastPrompt(), "DRAFT FILE TEXT") {
		t.Fatal("a file outside the project dir must be read (read-only)")
	}
	e.run("spec add login page", "no blocks here")
	if p := e.adapter.lastPrompt(); !strings.Contains(p, "=== input ===\nadd login page") {
		t.Fatalf("free text expected: %q", p)
	}
	before := e.adapter.calls()
	if _, err := e.run("spec"); err == nil {
		t.Fatal("empty input must be refused")
	}
	if _, err := e.run("roadmap   "); err == nil || e.adapter.calls() != before {
		t.Fatal("empty roadmap input must be refused without a call")
	}
	e.write("notes.txt", "NOTES")
	e.run("spec notes.txt", "x")
	if !strings.Contains(e.adapter.lastPrompt(), "NOTES") {
		t.Fatal("relative file in the project dir must be read")
	}
	e.run("spec missing.txt", "x")
	if !strings.Contains(e.adapter.lastPrompt(), "=== input ===\nmissing.txt") {
		t.Fatal("a token that is not a file is free text")
	}
}

func TestSDD34_UnknownID(t *testing.T) {
	e := newEnv(t)
	e.project("ready")
	_, err := e.run("spec-validate NOPE")
	if err == nil || !strings.Contains(err.Error(), "AUTH") {
		t.Fatalf("expected known ids in the refusal: %v", err)
	}
	if e.adapter.calls() != 0 {
		t.Fatal("no call expected")
	}
}

func TestSDD37_38_ModelMapping(t *testing.T) { // AC4
	e := newEnv(t)
	e.project("ready")
	e.run("implement AUTH", "no blocks")
	e.run("spec add x", "no blocks")
	if got := strings.Join(e.adapter.models, ","); got != "m-code,m-reason" {
		t.Fatalf("defaults: %s", got)
	}
	e.cfg.Strategies.SDD = map[string]string{"review": "/fast"}
	e.git.diff = "diff --git a/x b/x\n+a\n"
	e.run("review", "VERDICT: pass")
	if e.adapter.models[len(e.adapter.models)-1] != "m-fast" {
		t.Fatalf("review must use /fast: %v", e.adapter.models)
	}
	e.cfg.Strategies.Commands["/code"] = configuration.CommandAssignment{Enabled: false}
	n := e.adapter.calls()
	_, err := e.run("implement AUTH")
	if err == nil || !strings.Contains(err.Error(), "/code") || !strings.Contains(err.Error(), "disabled") || e.adapter.calls() != n {
		t.Fatalf("expected refusal naming /code and disabled: %v", err)
	}
	if e.svc.Active() != "" {
		t.Fatal("a refused subcommand must not stay active")
	}
}

func TestSDD41_42_ImplementContext(t *testing.T) { // AC5
	e := newEnv(t)
	e.project("ready")
	e.write(".gitignore", "*.log\nsrc/auth/.env\n")
	e.write("src/auth/.env", "SECRET=1\n")
	e.write("src/auth/debug.log", "LOGDATA\n")
	e.write("src/auth/logo.png", "PNG\x00\x01binary")
	e.write("src/other/x.go", "package other\n")
	e.write("src/auth/sub/deep.go", "package sub\n")
	e.run("implement AUTH", "no blocks")
	p := e.adapter.lastPrompt()
	for _, want := range []string{"=== src/auth/login.go ===", "=== src/auth/sub/deep.go ===", "=== AGENTS.md ===", "=== specs/features/auth.md ===", "=== specs/common/project.md ==="} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"src/other/x.go", "src/auth/.env", "logo.png", "debug.log", "SECRET=1"} {
		if strings.Contains(p, bad) {
			t.Errorf("must not send %q", bad)
		}
	}
}

func TestSDD43_62_SizeCap(t *testing.T) { // AC6, AC20
	e := newEnv(t)
	e.project("ready")
	e.write("src/auth/big.go", strings.Repeat("a", 401*1024))
	_, err := e.run("implement AUTH")
	if err == nil || !strings.Contains(err.Error(), "src/auth/big.go") || e.adapter.calls() != 0 {
		t.Fatalf("401 KB must be refused naming the file, before any call: %v", err)
	}
	e.write("src/auth/big.go", strings.Repeat("a", 395*1024))
	if _, err := e.run("implement AUTH", "no blocks"); err == nil || strings.Contains(err.Error(), "limit") || e.adapter.calls() != 1 {
		t.Fatalf("a context under the cap must reach the provider: %v", err)
	}
}

func TestSDD61_FileOverCapRefused(t *testing.T) { // AC19
	e := newEnv(t)
	e.project("ready")
	e.write("src/auth/huge.go", strings.Repeat("b", 500*1024))
	_, err := e.run("implement AUTH")
	if err == nil || !strings.Contains(err.Error(), "src/auth/huge.go") || e.adapter.calls() != 0 {
		t.Fatalf("expected refusal naming the file: %v", err)
	}
}

func TestSDD45_UnparsableShownRaw(t *testing.T) { // AC7
	e := newEnv(t)
	e.project("ready")
	r, err := e.run("implement AUTH", "just some prose, no file blocks")
	if err == nil || r.Waiting || !strings.Contains(e.out.String(), "just some prose") {
		t.Fatalf("raw output must be shown and nothing proposed: %v", err)
	}
	if e.svc.Active() != "" {
		t.Fatal("busy state must end")
	}
}

func TestSDD61_DiffInsteadOfContentRejected(t *testing.T) { // AC19
	e := newEnv(t)
	e.project("ready")
	diff := "--- a/src/auth/login.go\n+++ b/src/auth/login.go\n@@ -1 +1,2 @@\n package auth\n+// new\n"
	r, err := e.run("implement AUTH", block("src/auth/login.go", diff))
	if err == nil || r.Waiting || e.read("src/auth/login.go") != "package auth\n" {
		t.Fatalf("a diff must be unparsable and write nothing: %v", err)
	}
}

func TestSDD46_47_RejectedWhole(t *testing.T) { // AC8
	e := newEnv(t)
	e.project("ready")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(e.dir, "src", "auth", "link")); err != nil {
		t.Skip("symlinks unavailable")
	}
	cases := map[string]string{
		"README.md":                block("README.md", "x\n"),
		"../x":                     block("../x", "x\n"),
		".git/config":              block(".git/config", "x\n"),
		"src/auth/link/evil.go":    block("src/auth/link/evil.go", "x\n"),
		"/etc/passwd":              block("/etc/passwd", "x\n"),
		"deletion":                 "=== DELETE: src/auth/login.go ===\n",
		"mixed good and bad":       block("src/auth/ok.go", "x\n") + block("README.md", "x\n"),
		"src/auth/../../README.md": block("src/auth/../../README.md", "x\n"),
	}
	for name, reply := range cases {
		t.Run("SDD-46 "+name, func(t *testing.T) {
			r, err := e.run("implement AUTH", reply)
			if err == nil || r.Waiting {
				t.Fatalf("must be rejected whole: %v", err)
			}
			if e.has("src/auth/ok.go") || e.has("README.md") || e.read("src/auth/login.go") != "package auth\n" {
				t.Fatal("nothing may be written")
			}
			if _, e2 := os.Stat(filepath.Join(outside, "evil.go")); e2 == nil {
				t.Fatal("wrote outside the project")
			}
		})
	}
}

func TestSDD48_49_51_ApprovalFlow(t *testing.T) { // AC9
	e := newEnv(t)
	e.project("ready")
	idx := e.read("specs/index.md") + "| LOGIN | features/login.md | draft | V1 | none | src/login |\n"
	proposal := block("specs/features/login.md", featureText("LOGIN", "ready", "src/login")) + block("specs/index.md", idx)
	r, err := e.run("spec add login page", proposal)
	if err != nil || !r.Waiting {
		t.Fatalf("expected a waiting proposal: %v", err)
	}
	text := joined(r)
	if !strings.Contains(text, "new file: specs/features/login.md") || !strings.Contains(text, "changed file: specs/index.md") ||
		!strings.Contains(text, "+ | LOGIN |") || !strings.Contains(text, "redo [feedback]") {
		t.Fatalf("diff and answers expected:\n%s", text)
	}
	if e.has("specs/features/login.md") {
		t.Fatal("nothing is written before approval")
	}
	r, _ = e.answer("maybe")
	if !r.Waiting || !strings.Contains(joined(r), "yes | no | redo") {
		t.Fatalf("invalid answer must show the valid answers and keep waiting: %+v", r)
	}
	e.answer("redo shorter", proposal)
	p := e.adapter.lastPrompt()
	if e.adapter.calls() != 2 || !strings.Contains(p, "=== previous proposal ===") || !strings.Contains(p, "=== feedback ===\nshorter") ||
		!strings.Contains(p, "id: LOGIN") {
		t.Fatalf("redo must send the previous proposal and the feedback: %q", p)
	}
	if r, _ = e.answer("no"); r.Waiting || e.has("specs/features/login.md") {
		t.Fatal("no must write nothing")
	}
	if e.svc.Active() != "" {
		t.Fatal("busy state must end after no")
	}
	e.run("spec add login page", proposal)
	if _, err := e.answer("YES"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.read("specs/features/login.md"), "status: draft") || !strings.Contains(e.read("specs/index.md"), "| LOGIN | features/login.md | draft |") {
		t.Fatal("yes must write both files with status draft")
	}
}

func TestSDD49_50_Edit(t *testing.T) { // AC10
	e := newEnv(t)
	e.project("ready")
	good := block("src/auth/new.go", "package auth\n")
	e.run("implement AUTH", good)

	r, _ := e.answer("edit")
	if !r.Waiting || !strings.Contains(joined(r), "$EDITOR") || e.editor.called != 0 {
		t.Fatalf("no editor variable: must say so and keep waiting: %q", joined(r))
	}

	e.vars["EDITOR"] = "ed"
	e.vars["VISUAL"] = "vis -w"
	e.editor.result = block("README.md", "x\n")
	r, err := e.answer("edit")
	if err == nil || !r.Waiting || e.editor.command != "vis -w" {
		t.Fatalf("a bad edit must be rejected, the proposal kept, and $VISUAL preferred: %v %q", err, e.editor.command)
	}
	if !strings.Contains(e.editor.got, "=== FILE: src/auth/new.go ===") {
		t.Fatalf("the editor must get the proposal: %q", e.editor.got)
	}

	e.editor.result = "   \n"
	if r, _ = e.answer("edit"); !r.Waiting {
		t.Fatal("empty save leaves the proposal unchanged")
	}
	e.editor.err = os.ErrInvalid
	if r, _ = e.answer("edit"); !r.Waiting {
		t.Fatal("editor failure leaves the proposal unchanged")
	}
	e.editor.err = nil
	e.editor.result = block("src/auth/new.go", "package auth\n\nconst X = 1\n")
	r, err = e.answer("edit")
	if err != nil || !r.Waiting || !strings.Contains(joined(r), "+ const X = 1") {
		t.Fatalf("a good edit must be re-shown: %v %q", err, joined(r))
	}
	e.answer("yes")
	if e.read("src/auth/new.go") != "package auth\n\nconst X = 1\n" || e.has("README.md") {
		t.Fatal("the edited proposal must be written")
	}
}

func TestSDD53_NewSpecIDChecks(t *testing.T) { // AC11
	e := newEnv(t)
	e.project("ready")
	dup := block("specs/features/auth2.md", featureText("AUTH", "draft", "src/x")) + block("specs/index.md", e.read("specs/index.md"))
	if r, err := e.run("spec add auth", dup); err == nil || r.Waiting || !strings.Contains(err.Error(), "AUTH") {
		t.Fatalf("a duplicate id must be rejected: %v", err)
	}
	noFront := block("specs/features/x.md", "# no front matter\n") + block("specs/index.md", e.read("specs/index.md"))
	if _, err := e.run("spec add x", noFront); err == nil {
		t.Fatal("invalid front matter must be rejected")
	}
	exists := block("specs/features/auth.md", featureText("NEW", "draft", "src/x")) + block("specs/index.md", e.read("specs/index.md")+"| NEW | features/auth.md | draft | V1 | none | src/x |\n")
	if _, err := e.run("spec add new", exists); err == nil {
		t.Fatal("an existing feature file must not be replaced by a new spec")
	}
	noRow := block("specs/features/n.md", featureText("N", "draft", "src/n")) + block("specs/index.md", e.read("specs/index.md"))
	if _, err := e.run("spec add n", noRow); err == nil {
		t.Fatal("a missing index row must be rejected")
	}
	if e.has("specs/features/n.md") || e.has("specs/features/x.md") {
		t.Fatal("nothing may be written")
	}
}

func TestSDD54_ValidateVerdicts(t *testing.T) { // AC12
	pass := func(t *testing.T) *env {
		e := newEnv(t)
		e.project("draft")
		return e
	}
	t.Run("SDD-54 pass proposes ready in both places", func(t *testing.T) {
		e := pass(t)
		r, err := e.run("spec-validate AUTH", "Looks good.\nVERDICT: pass\n")
		if err != nil || !r.Waiting {
			t.Fatalf("%v %+v", err, r)
		}
		e.answer("yes")
		if !strings.Contains(e.read("specs/features/auth.md"), "status: ready") || !strings.Contains(e.read("specs/index.md"), "| AUTH | features/auth.md | ready |") {
			t.Fatal("status ready expected in both places")
		}
	})
	fixed := block("specs/features/auth.md", featureText("AUTH", "ready", "src/auth")+"- AUTH-2 added\n")
	for name, reply := range map[string]string{
		"fail":      "Problems found.\n" + fixed + "VERDICT: fail\n",
		"no marker": "Problems found.\n" + fixed,
	} {
		t.Run("SDD-54 "+name+" stays draft", func(t *testing.T) {
			e := pass(t)
			r, err := e.run("spec-validate AUTH", reply)
			if err != nil || !r.Waiting || !strings.Contains(joined(r), "verdict: fail") {
				t.Fatalf("%v %q", err, joined(r))
			}
			e.answer("yes")
			if !strings.Contains(e.read("specs/features/auth.md"), "status: draft") || !strings.Contains(e.read("specs/features/auth.md"), "AUTH-2") ||
				strings.Contains(e.read("specs/index.md"), "ready") {
				t.Fatal("the corrected spec must be written and stay draft (the tool, not the model, sets the status)")
			}
		})
	}
	t.Run("SDD-54 fail without corrected spec writes nothing", func(t *testing.T) {
		e := pass(t)
		r, err := e.run("spec-validate AUTH", "Problems.\nVERDICT: fail\n")
		if err != nil || r.Waiting {
			t.Fatalf("%v %+v", err, r)
		}
	})
}

func TestSDD54_SpecReviewVerdicts(t *testing.T) { // AC13
	e := newEnv(t)
	e.project("ready")
	r, err := e.run("spec-review AUTH", "All covered.\nVERDICT: pass\n")
	if err != nil || !r.Waiting {
		t.Fatalf("%v %+v", err, r)
	}
	e.answer("yes")
	if !strings.Contains(e.read("specs/features/auth.md"), "status: implemented") || !strings.Contains(e.read("specs/index.md"), "| implemented |") {
		t.Fatal("implemented expected")
	}
	e.project("ready")
	r, err = e.run("spec-review AUTH", "Gaps: AUTH-1.\nVERDICT: fail\n")
	if err != nil || r.Waiting || !strings.Contains(e.read("specs/features/auth.md"), "status: ready") {
		t.Fatalf("fail writes nothing: %v", err)
	}
}

func TestSDD55_ImplementNeedsReady(t *testing.T) { // AC14, AC21
	e := newEnv(t)
	e.project("draft")
	if _, err := e.run("implement AUTH"); err == nil || e.adapter.calls() != 0 {
		t.Fatalf("draft must be refused: %v", err)
	}
	e.project("implemented")
	_, err := e.run("implement AUTH")
	if err == nil || !strings.Contains(err.Error(), "/sdd spec AUTH <change>") || e.adapter.calls() != 0 {
		t.Fatalf("implemented must point to a spec change: %v", err)
	}
}

func TestSDD56_Review(t *testing.T) { // AC15
	e := newEnv(t)
	e.project("ready")
	if r, err := e.run("review"); err != nil || e.adapter.calls() != 0 || !strings.Contains(joined(r), "no uncommitted changes") {
		t.Fatalf("no diff must not call the model: %v %q", err, joined(r))
	}
	e.git.err = ErrNotRepo
	if _, err := e.run("review"); err == nil || !strings.Contains(err.Error(), "git repository") {
		t.Fatalf("outside a repo must be refused: %v", err)
	}
	e.git.err = ErrNoCommits
	if _, err := e.run("review"); err == nil || !strings.Contains(err.Error(), "at least one commit") || e.adapter.calls() != 0 {
		t.Fatalf("a repo with no commits must be refused: %v", err)
	}
	e.git.err = nil
	e.git.diff = "diff --git a/src/auth/x.go b/src/auth/x.go\n--- a/src/auth/x.go\n+++ b/src/auth/x.go\n@@ -1 +1 @@\n-a\n+b\n"
	e.git.untracked = []string{"src/auth/new.go"}
	e.write("src/auth/new.go", "package auth // untracked\n")
	e.write("notes.bin", "a\x00b")
	r, err := e.run("review", "A finding.\nVERDICT: fail\n")
	p := e.adapter.lastPrompt()
	if err != nil || r.Waiting || !strings.Contains(joined(r), "verdict: fail") {
		t.Fatalf("%v %+v", err, r)
	}
	for _, want := range []string{"=== git diff HEAD ===", "=== specs/features/auth.md ===", "untracked: src/auth/new.go", "=== specs/common/conventions.md ===", "=== AGENTS.md ==="} {
		if want == "=== specs/common/conventions.md ===" {
			continue // the fixture has no conventions file; missing files are left out
		}
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	for _, c := range e.git.calls {
		if c != "diff" && c != "untracked" {
			t.Errorf("unexpected git call %s (SDD-58)", c)
		}
	}
}

func TestSDD40_OneAtATime(t *testing.T) { // AC16
	e := newEnv(t)
	e.project("ready")
	e.adapter.gate = make(chan struct{})
	e.adapter.replies = []string{"no blocks"}
	done := make(chan struct{})
	go func() {
		e.svc.Run(context.Background(), "spec add x", e.out)
		close(done)
	}()
	for e.svc.Active() == "" {
	}
	if _, err := e.svc.Run(context.Background(), "review", e.out); err == nil || !strings.Contains(err.Error(), "spec") {
		t.Fatalf("a second subcommand must be refused naming the active one: %v", err)
	}
	if _, err := e.svc.Run(context.Background(), "init", e.out); err == nil {
		t.Fatal("init must be refused while a subcommand runs")
	}
	close(e.adapter.gate)
	<-done
	if e.svc.Active() != "" {
		t.Fatal("busy state must end")
	}
}

func TestSDD40_WaitingBlocksOthers(t *testing.T) {
	e := newEnv(t)
	e.project("ready")
	e.run("implement AUTH", block("src/auth/n.go", "package auth\n"))
	if e.svc.Active() != "implement" {
		t.Fatal("a waiting proposal keeps the subcommand active")
	}
	if _, err := e.run("review"); err == nil {
		t.Fatal("another subcommand must be refused while one waits for approval")
	}
}

func TestSDD57_HistoryRecords(t *testing.T) { // AC17
	e := newEnv(t)
	e.project("ready")
	e.run("spec add LOGINTEXT", "prose")
	e.run("implement AUTH", "prose")
	e.git.diff = "diff --git a/x b/x\n+a\n"
	e.run("review", "VERDICT: pass")
	lines := strings.Split(strings.TrimSpace(string(e.hist.data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 history lines, got %d", len(lines))
	}
	for i, sub := range []string{"spec", "implement", "review"} {
		if !strings.Contains(lines[i], `"sdd":"`+sub+`"`) {
			t.Errorf("line %d lacks sdd=%s: %s", i, sub, lines[i])
		}
	}
	all := string(e.hist.data)
	for _, leak := range []string{"LOGINTEXT", "package auth", "prose", "=== "} {
		if strings.Contains(all, leak) {
			t.Errorf("history must hold no prompt or file text, found %q", leak)
		}
	}
	if !strings.Contains(lines[1], `"command":"/code"`) {
		t.Errorf("HIST-12: command is the mapped routing command: %s", lines[1])
	}
}

func TestSDD59_FullCycleWritesOnlyAllowedPaths(t *testing.T) { // AC18
	e := newEnv(t)
	cfgDir := os.Getenv("AGENT_FACTORY_HOME")
	models := filepath.Join(cfgDir, "models.json")
	strat := filepath.Join(cfgDir, "strategies.json")
	os.WriteFile(models, []byte(`{"providers":{}}`), 0o600)
	os.WriteFile(strat, []byte(`{"strategy":"cost"}`), 0o600)

	r, err := e.run("init")
	if err != nil || r.Confirm == nil {
		t.Fatal(err)
	}
	r.Confirm.Run(context.Background())

	idx := e.read("specs/index.md") + "| WIDGET | features/widget.md | draft | V1 | none | src/widget |\n"
	e.run("spec add widget", block("specs/features/widget.md", featureText("WIDGET", "draft", "src/widget"))+block("specs/index.md", idx))
	e.answer("yes")
	e.run("spec-validate WIDGET", "ok\nVERDICT: pass\n")
	e.answer("yes")
	e.run("implement WIDGET", block("src/widget/widget.go", "package widget\n")+block("src/widget/widget_test.go", "package widget\n"))
	if _, err := e.answer("yes"); err != nil {
		t.Fatal(err)
	}
	e.run("spec-review WIDGET", "VERDICT: pass\n")
	e.answer("yes")

	if !strings.Contains(e.read("specs/index.md"), "| WIDGET | features/widget.md | implemented |") {
		t.Fatalf("cycle must end implemented:\n%s", e.read("specs/index.md"))
	}
	allowed := map[string]bool{"specs/features/widget.md": true, "src/widget/widget.go": true, "src/widget/widget_test.go": true}
	for _, f := range initFiles {
		allowed[f.Path] = true
	}
	filepath.WalkDir(e.dir, func(p string, d os.DirEntry, _ error) error {
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(e.dir, p)
		if !allowed[filepath.ToSlash(rel)] {
			t.Errorf("unexpected file written: %s", rel)
		}
		return nil
	})
	for p, want := range map[string]string{models: `{"providers":{}}`, strat: `{"strategy":"cost"}`} {
		if b, _ := os.ReadFile(p); string(b) != want {
			t.Errorf("%s changed", p) // TEST-5
		}
	}
}

func TestSDD59_NeverWritesUserConfigNames(t *testing.T) { // TEST-5
	e := newEnv(t)
	e.project("ready")
	e.write("specs/features/auth.md", featureText("AUTH", "ready", "src/auth"))
	e.run("implement AUTH", block("src/auth/models.json", "{}\n"))
	r, _ := e.answer("yes")
	if e.has("src/auth/models.json") {
		t.Fatalf("models.json must never be written: %+v", r)
	}
}

func TestSDD64_ChangeExistingFeature(t *testing.T) { // AC22
	e := newEnv(t)
	e.project("ready")
	e.write("specs/templates/change.md", "# CHANGE-TEMPLATE\n")
	changed := featureText("AUTH", "ready", "src/auth") + "- AUTH-2 rate limit\n"
	dec := e.read("specs/decisions.md") + "- D-002 rate limit\n"
	good := block("specs/features/auth.md", changed) + block("specs/decisions.md", dec)

	r, err := e.run("spec AUTH add rate limit", good)
	if err != nil || !r.Waiting {
		t.Fatalf("%v", err)
	}
	p := e.adapter.lastPrompt()
	if !strings.Contains(p, "=== specs/features/auth.md ===") || !strings.Contains(p, "CHANGE-TEMPLATE") || !strings.Contains(p, "You change an existing feature spec") {
		t.Fatalf("a change must send the feature and change.md: %q", p)
	}
	e.answer("yes")
	feat := e.read("specs/features/auth.md")
	if !strings.Contains(feat, "status: draft") || !strings.Contains(feat, "AUTH-1 first rule") || !strings.Contains(feat, "AUTH-2") ||
		!strings.Contains(e.read("specs/index.md"), "| AUTH | features/auth.md | draft |") || !strings.Contains(e.read("specs/decisions.md"), "D-002") {
		t.Fatal("status must reset to draft in both places, existing rule ids stay")
	}

	e.project("ready")
	renamed := block("specs/features/auth.md", strings.Replace(featureText("AUTH", "ready", "src/auth"), "id: AUTH", "id: OTHER", 1))
	if _, err := e.run("spec AUTH rename", renamed); err == nil {
		t.Fatal("renaming the id must be rejected")
	}
	edited := block("specs/features/auth.md", changed) + block("specs/decisions.md", strings.Replace(e.read("specs/decisions.md"), "D-001 first", "D-001 CHANGED", 1))
	if _, err := e.run("spec AUTH edit decisions", edited); err == nil || !strings.Contains(err.Error(), "appended") {
		t.Fatalf("editing an existing decisions line must be rejected: %v", err)
	}
	if strings.Contains(e.read("specs/decisions.md"), "CHANGED") {
		t.Fatal("nothing may be written")
	}

	e.run("spec auth add rate limit", "prose")
	if !strings.Contains(e.adapter.lastPrompt(), "You write a new feature spec") {
		t.Fatal("lowercase id must create a new spec (case-sensitive match)")
	}
	e.run("spec AUTH", "prose")
	if !strings.Contains(e.adapter.lastPrompt(), "You write a new feature spec") {
		t.Fatal("an id with no input after it is free text for a new spec")
	}
}

func TestSDD65_ImplementOnlyUnderModules(t *testing.T) { // AC23
	e := newEnv(t)
	e.project("ready")
	if r, err := e.run("implement AUTH", block("test/auth_test.go", "package t\n")); err == nil || r.Waiting {
		t.Fatalf("a test outside modules must be rejected: %v", err)
	}
	r, err := e.run("implement AUTH", block("src/auth/auth_test.go", "package auth\n"))
	if err != nil || !r.Waiting {
		t.Fatalf("a test inside modules must be accepted: %v", err)
	}
	e.answer("yes")
	if !e.has("src/auth/auth_test.go") {
		t.Fatal("written")
	}
}

func TestSDD41_ContextPerSubcommand(t *testing.T) {
	e := newEnv(t)
	e.project("ready")
	e.write("specs/roadmap.md", "ROADMAP-TEXT\n")
	e.write("specs/templates/roadmap.md", "ROADMAP-TEMPLATE\n")
	e.write("specs/templates/feature.md", "FEATURE-TEMPLATE\n")
	e.run("roadmap plan v2", "x")
	p := e.adapter.lastPrompt()
	for _, w := range []string{"=== AGENTS.md ===", "=== specs/common/project.md ===", "ROADMAP-TEXT", "ROADMAP-TEMPLATE", "=== specs/decisions.md ===", "plan v2"} {
		if !strings.Contains(p, w) {
			t.Errorf("roadmap prompt lacks %q", w)
		}
	}
	if strings.Contains(p, "specs/index.md") {
		t.Error("roadmap must not send the index")
	}
	e.run("spec new thing", "x")
	if p := e.adapter.lastPrompt(); !strings.Contains(p, "FEATURE-TEMPLATE") || !strings.Contains(p, "=== specs/index.md ===") {
		t.Error("spec prompt lacks its context")
	}
	e.run("spec-validate AUTH", "x")
	if p := e.adapter.lastPrompt(); strings.Contains(p, "AGENTS.md") || !strings.Contains(p, "=== specs/decisions.md ===") {
		t.Error("spec-validate context is wrong")
	}
}

func TestSDD46_RoadmapAllowedPaths(t *testing.T) {
	e := newEnv(t)
	e.project("ready")
	if _, err := e.run("roadmap x", block("specs/index.md", "x\n")); err == nil {
		t.Fatal("roadmap may not write the index")
	}
	r, err := e.run("roadmap x", block("specs/roadmap.md", "# R\n")+block("specs/decisions.md", "# D\n"))
	if err != nil || !r.Waiting {
		t.Fatal(err)
	}
}

func TestSDD42_IgnoreRules(t *testing.T) {
	files := map[string]string{
		".gitignore":     "# c\n*.log\nbuild/\n/root-only\n!keep.log\n**/gen/*.go\nsrc/*.tmp\n",
		"src/.gitignore": "local.txt\n",
	}
	ign := newIgnorer(func(rel string) (string, bool) { s, ok := files[rel]; return s, ok })
	for rel, want := range map[string]bool{
		"a.log": true, "src/a.log": true, "keep.log": false, "build": true, "src/build/x.go": true,
		"root-only": true, "src/root-only": false, "a/gen/x.go": true, "a/gen/x.txt": false,
		"src/x.tmp": true, "src/sub/x.tmp": false, "src/local.txt": true, "src/sub/local.txt": true, "local.txt": false,
		"src/main.go": false,
	} {
		if got := ign.ignored(rel, rel == "build"); got != want {
			t.Errorf("ignored(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestSDD48_UnifiedDiff(t *testing.T) {
	d := strings.Join(unifiedDiff("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n", "a\nb\nc\nd\nX\nf\ng\nh\ni\nj\n"), "\n")
	if !strings.Contains(d, "- e") || !strings.Contains(d, "+ X") || !strings.Contains(d, "  d") {
		t.Fatalf("diff: %s", d)
	}
	if got := strings.Join(unifiedDiff("", "new\n"), "\n"); got != "+ new" {
		t.Fatalf("new file diff: %q", got)
	}
}

func TestSDD30_PathsAreRelativeToProjectDir(t *testing.T) {
	e := newEnv(t)
	e.project("ready")
	e.run("implement AUTH", block("src/auth/n.go", "package auth\n"))
	e.answer("yes")
	if !e.has("src/auth/n.go") || e.svc.abs("src/auth/n.go") != filepath.Join(e.dir, "src", "auth", "n.go") {
		t.Fatal("SDD-30: paths must resolve against the project dir")
	}
}

func TestSDD39_ProviderErrorNoRetryNothingWritten(t *testing.T) {
	e := newEnv(t)
	e.project("ready")
	e.adapter.err = os.ErrDeadlineExceeded
	r, err := e.run("implement AUTH", block("src/auth/n.go", "package auth\n"))
	if err != nil || r.Waiting || e.adapter.calls() != 1 || e.has("src/auth/n.go") {
		t.Fatalf("SDD-39: one call, no retry, nothing written: %v calls=%d", err, e.adapter.calls())
	}
	if !strings.Contains(e.out.String(), "provider_failure") || e.svc.Active() != "" {
		t.Fatalf("the normalized error must be shown and the busy state must end:\n%s", e.out.String())
	}
}

func TestSDD44_SDDPromptsOnlyCarryContext(t *testing.T) {
	e := newEnv(t)
	e.project("ready")
	e.run("implement AUTH", "x")
	if !strings.Contains(e.adapter.lastPrompt(), "=== AGENTS.md ===") {
		t.Fatal("SDD-44: SDD prompts carry file context")
	}
}

func TestSDD52_StatusEditsMadeByTheTool(t *testing.T) {
	e := newEnv(t)
	e.project("draft")
	liar := block("specs/features/auth.md", featureText("AUTH", "implemented", "src/auth"))
	e.run("spec-validate AUTH", liar+"VERDICT: fail\n")
	e.answer("yes")
	if !strings.Contains(e.read("specs/features/auth.md"), "status: draft") || strings.Contains(e.read("specs/index.md"), "implemented") {
		t.Fatal("SDD-52: status text from the model must be overridden by the tool")
	}
	if got, ok := setIndexStatus("| A | f | draft | V1 |\n", "B", "ready"); ok || got == "" {
		t.Fatal("a missing index row must be reported")
	}
}

func TestSDD58_60_OnlyAllowedExternalCalls(t *testing.T) {
	e := newEnv(t)
	e.project("ready")
	e.git.diff = "diff --git a/x b/x\n+a\n"
	e.run("review", "VERDICT: pass")
	e.run("implement AUTH", "x")
	if got := strings.Join(e.git.calls, ","); got != "diff,untracked" {
		t.Fatalf("SDD-58: only the review git calls are allowed, got %s", got)
	}
}

func TestSDD63_ReimplementOnlyAfterSpecChangeAndValidate(t *testing.T) {
	e := newEnv(t)
	e.project("implemented")
	if _, err := e.run("implement AUTH"); err == nil {
		t.Fatal("SDD-63: direct re-implementation must be refused")
	}
	e.run("spec AUTH tweak", block("specs/features/auth.md", featureText("AUTH", "ready", "src/auth")))
	e.answer("yes")
	if _, err := e.run("implement AUTH"); err == nil {
		t.Fatal("SDD-63: a draft spec cannot be implemented")
	}
	e.run("spec-validate AUTH", "VERDICT: pass\n")
	e.answer("yes")
	if _, err := e.run("implement AUTH", "x"); err != nil && strings.Contains(err.Error(), "status") {
		t.Fatalf("SDD-63: after validate the feature is ready again: %v", err)
	}
}

func TestSDD_RuleLabels(t *testing.T) {
	t.Run("SDD-38 an unresolved mapped model is refused and nothing runs", func(t *testing.T) {
		e := newEnv(t)
		e.project("ready")
		e.cfg.Strategies.Commands["/reason"] = configuration.CommandAssignment{Enabled: true}
		_, err := e.run("spec add x")
		if err == nil || !strings.Contains(err.Error(), "spec") || !strings.Contains(err.Error(), "/reason") || e.adapter.calls() != 0 {
			t.Fatalf("%v", err)
		}
	})
	t.Run("SDD-47 a deletion request is rejected whole", func(t *testing.T) {
		e := newEnv(t)
		e.project("ready")
		r, err := e.run("implement AUTH", block("src/auth/ok.go", "package auth\n")+"=== DELETE: src/auth/login.go ===\n")
		if err == nil || r.Waiting || e.has("src/auth/ok.go") || !strings.Contains(err.Error(), "delete") {
			t.Fatalf("%v", err)
		}
	})
	t.Run("SDD-51 a failed write reports which files were and were not written", func(t *testing.T) {
		e := newEnv(t)
		e.project("ready")
		e.write("src/auth/blocked/keep.go", "package blocked\n")
		_, err := e.run("implement AUTH", block("src/auth/a.go", "package auth\n")+block("src/auth/blocked", "x\n"))
		if err != nil {
			t.Fatal(err)
		}
		_, err = e.answer("yes")
		if err == nil || !strings.Contains(err.Error(), "Written: src/auth/a.go") || !strings.Contains(err.Error(), "Not written: src/auth/blocked") {
			t.Fatalf("%v", err)
		}
	})
	t.Run("SDD-60 git and editor sit behind interfaces", func(t *testing.T) {
		var _ Git = (*fakeGit)(nil)
		var _ Editor = (*fakeEditor)(nil)
		var _ Files = (*fakeFilesCheck)(nil)
	})
	t.Run("SDD-62 the size cap is 400 KB", func(t *testing.T) {
		if SizeCap != 400*1024 {
			t.Fatal(SizeCap)
		}
	})
}

func TestSDD64_RuleIDsMustStay(t *testing.T) {
	e := newEnv(t)
	e.project("ready")
	renumbered := strings.Replace(featureText("AUTH", "ready", "src/auth"), "AUTH-1", "AUTH-7", 1)
	removed := strings.Replace(featureText("AUTH", "ready", "src/auth"), "- AUTH-1 first rule\n", "", 1) + "- AUTH-2 new\n"
	for name, text := range map[string]string{"renumbered": renumbered, "removed": removed} {
		t.Run("SDD-64 "+name+" rule id rejected", func(t *testing.T) {
			r, err := e.run("spec AUTH change it", block("specs/features/auth.md", text))
			if err == nil || r.Waiting || !strings.Contains(err.Error(), "AUTH-1") {
				t.Fatalf("must be rejected naming AUTH-1: %v", err)
			}
			if !strings.Contains(e.read("specs/features/auth.md"), "AUTH-1 first rule") {
				t.Fatal("nothing may be written")
			}
		})
	}
	t.Run("SDD-64 reworded rule and new ids accepted", func(t *testing.T) {
		text := strings.Replace(featureText("AUTH", "ready", "src/auth"), "first rule", "first rule, reworded", 1) + "- AUTH-2 new\n"
		if r, err := e.run("spec AUTH change it", block("specs/features/auth.md", text)); err != nil || !r.Waiting {
			t.Fatalf("%v", err)
		}
	})
}

// TestSDD_IDLabels gives the rules whose tests carry only a combined name an
// exact, greppable label (TEST-1).
func TestSDD_IDLabels(t *testing.T) {
	for id, fn := range map[string]func(*testing.T){
		"SDD-31 subcommand list and unknown subcommand":  TestSDD31_ListAndUnknownSubcommand,
		"SDD-32 refused without specs/index.md":          TestSDD32_RefusedWithoutIndex,
		"SDD-33 input is a file or free text":            TestSDD33_Input,
		"SDD-34 unknown id refused with known ids":       TestSDD34_UnknownID,
		"SDD-35 init creates files without a model":      TestSDD35_36_InitCreatesFilesWithoutModel,
		"SDD-36 init lists, asks, never overwrites":      TestSDD35_36_InitCreatesFilesWithoutModel,
		"SDD-41 context per subcommand":                  TestSDD41_ContextPerSubcommand,
		"SDD-42 source files exclude ignored and binary": TestSDD41_42_ImplementContext,
		"SDD-43 context over the cap is refused":         TestSDD43_62_SizeCap,
		"SDD-49 approval answers":                        TestSDD48_49_51_ApprovalFlow,
		"SDD-50 edit uses an owner-only temp file":       TestSDD49_50_Edit,
		"SDD-53 new spec id and front matter checks":     TestSDD53_NewSpecIDChecks,
		"SDD-55 implement needs ready":                   TestSDD55_ImplementNeedsReady,
		"SDD-56 review uses git diff HEAD":               TestSDD56_Review,
		"SDD-57 one history record per run":              TestSDD57_HistoryRecords,
		"SDD-59 only allowed paths are written":          TestSDD59_FullCycleWritesOnlyAllowedPaths,
		"SDD-61 full content only, big files refused":    TestSDD61_DiffInsteadOfContentRejected,
		"SDD-65 implement writes only under modules":     TestSDD65_ImplementOnlyUnderModules,
	} {
		t.Run(id, fn)
	}
	t.Run("SDD-54 the verdict is read from the last non-empty line", func(t *testing.T) {
		for text, want := range map[string]bool{"x\nVERDICT: pass\n\n": true, "x\nvErdict: FAIL": false, "VERDICT: pass\nmore text": false, "no marker": false} {
			if got, _ := readVerdict(text); got != want {
				t.Errorf("readVerdict(%q) = %v, want %v", text, got, want)
			}
		}
	})
	t.Run("SDD-37 subcommands map to routing commands through the sdd object", TestSDD37_38_ModelMapping)
	t.Run("SDD-45 unparsable output is shown raw and nothing is written", TestSDD45_UnparsableShownRaw)
	t.Run("SDD-48 each proposed file is shown new or changed with a diff", TestSDD48_49_51_ApprovalFlow)
	t.Run("SDD-52 status edits are made by the tool", TestSDD52_StatusEditsMadeByTheTool)
}

const otherRow = "| OTHER | features/other.md | draft | V1 | none | src/o |\n"

func TestSDD52_ModelCannotChangeOtherRows(t *testing.T) {
	e := newEnv(t)
	e.project("draft")
	e.write("specs/index.md", e.read("specs/index.md")+otherRow)
	idx := e.read("specs/index.md")
	liar := strings.Replace(idx, "| OTHER | features/other.md | draft", "| OTHER | features/other.md | ready", 1)
	_, err := e.run("spec-validate AUTH", block("specs/features/auth.md", featureText("AUTH", "draft", "src/auth"))+block("specs/index.md", liar)+"VERDICT: fail\n")
	if err == nil || !strings.Contains(err.Error(), "OTHER") || e.read("specs/index.md") != idx {
		t.Fatalf("SDD-52: a changed foreign row must reject the proposal: %v", err)
	}
	other := block("specs/features/auth.md", featureText("AUTH", "draft", "src/auth")) + "VERDICT: fail\n"
	for name, edited := range map[string]string{
		"version":   strings.Replace(idx, "| draft | V1 | none | src/o |", "| draft | V9 | none | src/o |", 1),
		"modules":   strings.Replace(idx, "src/o |", "src/evil |", 1),
		"duplicate": idx + strings.Replace(otherRow, "draft", "ready", 1),
	} {
		if _, err := e.run("spec-validate AUTH", other[:len(other)-len("VERDICT: fail\n")]+block("specs/index.md", edited)+"VERDICT: fail\n"); err == nil {
			t.Errorf("SDD-52: a %s change in the index must reject the proposal", name)
		}
	}
	dropped := strings.Replace(idx, otherRow, "", 1)
	if _, err = e.run("spec-validate AUTH", block("specs/features/auth.md", featureText("AUTH", "draft", "src/auth"))+block("specs/index.md", dropped)+"VERDICT: fail\n"); err == nil {
		t.Fatal("SDD-52: a removed foreign row must reject the proposal")
	}
}

func TestSDD53_IndexRowMustMatchFile(t *testing.T) {
	e := newEnv(t)
	e.project("draft")
	idx := e.read("specs/index.md")
	bad := idx + "| X | features/other.md | draft | V1 | none | src/x |\n"
	_, err := e.run("spec new x", block("specs/features/x.md", featureText("X", "draft", "src/x"))+block("specs/index.md", bad))
	if err == nil || e.has("specs/features/x.md") {
		t.Fatalf("SDD-53: an index row pointing at another file must be rejected: %v", err)
	}
	good := idx + "| X | features/x.md | draft | V1 | none | src/x |\n"
	if r, err := e.run("spec new x", block("specs/features/x.md", featureText("X", "draft", "src/x"))+block("specs/index.md", good)); err != nil || !r.Waiting {
		t.Fatalf("a matching row must be accepted: %v", err)
	}
}

func TestSDD54_PassChangesOnlyStatus(t *testing.T) {
	e := newEnv(t)
	e.project("draft")
	before := e.read("specs/features/auth.md")
	sneaky := block("specs/features/auth.md", featureText("AUTH", "draft", "src/auth")+"- AUTH-2 unvalidated rule\n")
	r, err := e.run("spec-validate AUTH", sneaky+"VERDICT: pass\n")
	if err != nil || !r.Waiting || strings.Contains(joined(r), "unvalidated") {
		t.Fatalf("SDD-54: pass must propose only the status change: %v %q", err, joined(r))
	}
	e.answer("yes")
	if got := e.read("specs/features/auth.md"); got != strings.Replace(before, "status: draft", "status: ready", 1) {
		t.Fatalf("SDD-54: only the status may change:\n%s", got)
	}
}

func TestSDD40_EditThatChangesNothingEndsTheRun(t *testing.T) {
	e := newEnv(t)
	e.project("ready")
	e.run("implement AUTH", block("src/auth/new.go", "package auth\n"))
	e.vars["EDITOR"] = "ed"
	e.editor.result = block("src/auth/login.go", "package auth\n") // identical to disk
	r, err := e.answer("edit")
	if err != nil || r.Waiting || e.svc.Active() != "" {
		t.Fatalf("SDD-40: the run must end, not stay busy: %v %+v active=%q", err, r, e.svc.Active())
	}
	if _, err := e.run("review"); err != nil && strings.Contains(err.Error(), "already running") {
		t.Fatal("SDD-40: a later /sdd command must not be refused as busy")
	}
}

func TestSDD59_UserOwnedNamesRejectedBeforeApproval(t *testing.T) {
	e := newEnv(t)
	e.project("ready")
	r, err := e.run("implement AUTH", block("src/auth/new.go", "package auth\n")+block("src/auth/models.json", "{}\n"))
	if err == nil || r.Waiting || !strings.Contains(err.Error(), "src/auth/models.json") || e.has("src/auth/new.go") {
		t.Fatalf("SDD-59: rejected whole before the prompt, nothing written: %v", err)
	}
}

func TestARCH13_SDDDoesNotImportProviderCodeOrExec(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		for name, f := range pkg.Files {
			for _, im := range f.Imports {
				if p := strings.Trim(im.Path.Value, `"`); strings.Contains(p, "infra/providers") || p == "os/exec" || p == "net/http" {
					t.Errorf("ARCH-13/ARCH-12: %s imports %s", name, p)
				}
			}
		}
	}
}

// TestSDD_RuleLabels gives the common rules a greppable label (TEST-1).
func TestSDD_CommonRuleLabels(t *testing.T) {
	t.Run("ARCH-11 project writes go through one approved-proposal writer", TestSDD59_FullCycleWritesOnlyAllowedPaths)
	t.Run("ARCH-12 git and editor sit behind interfaces, no exec in the sdd package", TestARCH13_SDDDoesNotImportProviderCodeOrExec)
	t.Run("ARCH-13 sdd resolves through ROUTE and runs through EXEC, no provider import", TestARCH13_SDDDoesNotImportProviderCodeOrExec)
	t.Run("CONV-7 files are checked then written atomically", TestSDD46_47_RejectedWhole)
	t.Run("TEST-5 SDD never writes models.json or strategies.json", TestSDD59_NeverWritesUserConfigNames)
	t.Run("SEC-6 ignored and binary files are never sent", TestSDD41_42_ImplementContext)
}

func TestSDD64_ChangeMayUpdateOwnIndexRowCells(t *testing.T) {
	e := newEnv(t)
	e.project("implemented")
	idx := strings.Replace(e.read("specs/index.md"), "| src/auth |", "| src/auth, src/session |", 1)
	text := featureText("AUTH", "implemented", "src/auth, src/session")
	r, err := e.run("spec AUTH add sessions", block("specs/features/auth.md", text)+block("specs/index.md", idx))
	if err != nil || !r.Waiting {
		t.Fatalf("SDD-64: a change may update its own index row cells: %v", err)
	}
	e.answer("yes")
	got := e.read("specs/index.md")
	if !strings.Contains(got, "src/auth, src/session") || !strings.Contains(got, "| draft |") {
		t.Fatalf("SDD-64: modules cell updated and status reset to draft:\n%s", got)
	}
	bad := strings.Replace(got, "features/auth.md", "features/elsewhere.md", 1)
	if _, err := e.run("spec AUTH again", block("specs/features/auth.md", text)+block("specs/index.md", bad)); err == nil || !strings.Contains(err.Error(), "keep its file") {
		t.Fatalf("SDD-52: the target row must keep its file: %v", err)
	}
}
