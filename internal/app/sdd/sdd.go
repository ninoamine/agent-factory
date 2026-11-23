// Package sdd implements the /sdd workflow (SDD): scaffold a spec-driven
// project, build prompts with file context, run one model per subcommand, and
// write the model's proposal only after the user approves it. It resolves
// models through routing and runs them through execution (ARCH-13); it never
// imports provider code. All logic lives here, not in the cli (SDD-60, ARCH-3).
package sdd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/history"
	"github.com/ninoamine/agent-factory/internal/app/routing"
	"github.com/ninoamine/agent-factory/internal/domain"
)

// SizeCap is the most text one prompt may hold (SDD-62): 400 KB.
const SizeCap = 400 * 1024

// ErrNotRepo is returned by Git when the project dir is not a git repository (SDD-56).
var ErrNotRepo = errors.New("not a git repository")

// ErrNoCommits is returned by Git when the repository has no commit yet, so `git diff HEAD` cannot run (SDD-56).
var ErrNoCommits = errors.New("git repository has no commits")

// Files is the project-dir access the service needs. Implemented in infra/storage.
// Paths are absolute.
type Files interface {
	Read(path string) ([]byte, error)
	Head(path string, n int) ([]byte, error)
	Stat(path string) (fs.FileInfo, error)
	Lstat(path string) (fs.FileInfo, error)
	ReadDir(path string) ([]fs.DirEntry, error)
	EvalSymlinks(path string) (string, error)
	MkdirAll(path string) error
	WriteAtomic(ctx context.Context, path string, data []byte) error
}

// Git is the only external command SDD may run (ARCH-12, SDD-56). Implemented in infra/gitcli.
type Git interface {
	Diff(ctx context.Context, dir string) (string, error)
	Untracked(ctx context.Context, dir string) ([]string, error)
}

// Editor opens text in the user's editor and returns the saved text (SDD-50).
type Editor interface {
	Edit(ctx context.Context, command, content string) (string, error)
}

// Runner runs one prompt on one model (EXEC). Implemented by execution.Service.
type Runner interface {
	RunSDD(ctx context.Context, cfg configuration.Config, d routing.Decision, sdd string, out execution.Output) execution.Record
}

// Service handles /sdd. Dir is the project dir: the working directory at launch (SDD-30).
type Service struct {
	Dir    string
	Files  Files
	Git    Git
	Editor Editor
	Runner Runner
	// Config returns the active configuration and a release func for the run (REFRESH-9).
	Config func() (configuration.Config, func())
	Getenv func(string) string

	mu     sync.Mutex
	active string
	pend   *pending
}

// Reply is what the screen shows after /sdd or an approval answer.
type Reply struct {
	Lines   []string
	Confirm *history.Confirmation // /sdd init asks first (SDD-36, TUI-28)
	Waiting bool                  // a proposal waits for approval (SDD-49, TUI-25)
}

type pending struct {
	j     *job
	files []File // changed files only
	raw   string
}

// job is one prepared model run.
type job struct {
	sub      string // subcommand name (history, mapping)
	instr    string
	parts    []part
	decision routing.Decision
	id       string
	// build turns the proposed files into the files to write, checking paths
	// and applying status edits (SDD-46, SDD-52). Called again after `edit`.
	build func(files []File, pass bool) ([]File, error)
	pass  bool
	// redo state (SDD-49)
	prev, feedback string
}

type part struct {
	label string
	text  string
	size  int // set for files too big to read
}

var modelSubs = []string{"roadmap", "spec", "spec-validate", "implement", "spec-review", "review"}

var defaultCommand = map[string]domain.RoutingCommand{"implement": domain.CmdCode}

// ModelSubcommand returns the subcommand name when args name one that runs a
// model, else "". The screen uses it to track the busy state (SDD-40).
func ModelSubcommand(args string) string {
	sub, _ := splitFirst(strings.TrimSpace(args))
	for _, m := range modelSubs {
		if m == sub {
			return sub
		}
	}
	return ""
}

// Active returns the subcommand that is running or waiting for approval (SDD-40).
func (s *Service) Active() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (s *Service) begin(sub string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != "" {
		return fmt.Errorf("/sdd %s is already running or waiting for approval; finish it first", s.active)
	}
	s.active = sub
	return nil
}

func (s *Service) end() {
	s.mu.Lock()
	s.active, s.pend = "", nil
	s.mu.Unlock()
}

// settle ends the busy state unless a proposal now waits for approval.
func (s *Service) settle(r Reply, err error) (Reply, error) {
	if !r.Waiting {
		s.end()
	}
	return r, err
}

// Run handles `/sdd <args>` (SDD-31).
func (s *Service) Run(ctx context.Context, args string, out execution.Output) (Reply, error) {
	sub, rest := splitFirst(strings.TrimSpace(args))
	known := sub == "init"
	for _, m := range modelSubs {
		known = known || m == sub
	}
	if sub != "" && !known {
		return Reply{}, fmt.Errorf("unknown /sdd subcommand %q. Subcommands:\n%s", sub, subcommandList())
	}
	if err := s.begin(sub); err != nil {
		return Reply{}, err
	}
	switch sub {
	case "":
		return s.settle(s.list(), nil)
	case "init":
		return s.settle(s.init(ctx))
	}
	rows, err := s.loadRows()
	if err != nil {
		return s.settle(Reply{}, err)
	}
	j, reply, err := s.prepare(ctx, sub, rest, rows)
	if err != nil || j == nil {
		return s.settle(reply, err)
	}
	return s.settle(s.execute(ctx, j, out))
}

func (s *Service) list() Reply {
	state := "not initialized: run /sdd init"
	if s.exists(indexPath) {
		state = "initialized (specs/index.md found)"
	}
	return Reply{Lines: []string{"SDD subcommands:\n" + subcommandList(), "Project: " + state}}
}

func (s *Service) abs(rel string) string { return filepath.Join(s.Dir, filepath.FromSlash(rel)) }

func (s *Service) exists(rel string) bool {
	_, err := s.Files.Lstat(s.abs(rel))
	return err == nil
}

func (s *Service) read(rel string) (string, bool) {
	b, err := s.Files.Read(s.abs(rel))
	if err != nil {
		return "", false
	}
	return string(b), true
}

// loadRows requires an initialized project (SDD-32) and parses the index.
func (s *Service) loadRows() ([]IndexRow, error) {
	text, ok := s.read(indexPath)
	if !ok {
		return nil, errors.New("this project is not initialized: specs/index.md does not exist. Run /sdd init first")
	}
	return parseIndex(text), nil
}

// resolve maps the subcommand to a routing command and a model (SDD-37, SDD-38).
func (s *Service) resolve(sub string) (routing.Decision, error) {
	cfg, release := s.Config()
	release()
	cmd, ok := defaultCommand[sub]
	if !ok {
		cmd = domain.CmdReason
	}
	if cfg.Strategies != nil {
		if c, ok := cfg.Strategies.SDD[sub]; ok {
			cmd = domain.RoutingCommand(c)
		}
	}
	d := routing.Resolve(cfg, cmd)
	if d.Kind != routing.Routed {
		return d, fmt.Errorf("/sdd %s cannot run: it uses %s, which is not usable: %s", sub, cmd, d.Message)
	}
	return d, nil
}

// prepare validates the arguments and builds the prompt parts (SDD-33, SDD-34, SDD-41, SDD-55).
// A nil job with a nil error means there is nothing to run.
func (s *Service) prepare(ctx context.Context, sub, rest string, rows []IndexRow) (*job, Reply, error) {
	j := &job{sub: sub, instr: instructions[sub]}
	var err error
	switch sub {
	case "roadmap":
		err = s.prepRoadmap(j, rest)
	case "spec":
		err = s.prepSpec(j, rest, rows)
	case "spec-validate", "implement", "spec-review":
		err = s.prepFeature(j, rest, rows)
	case "review":
		var reply Reply
		reply, err = s.prepReview(ctx, j, rows)
		if err != nil || len(j.parts) == 0 {
			return nil, reply, err
		}
	}
	if err != nil {
		return nil, Reply{}, err
	}
	d, err := s.resolve(sub)
	if err != nil {
		return nil, Reply{}, err
	}
	j.decision = d
	return j, Reply{}, nil
}

func (s *Service) prepRoadmap(j *job, rest string) error {
	in, err := s.input(rest)
	if err != nil {
		return err
	}
	j.parts = s.parts("AGENTS.md", "specs/common/project.md", "specs/roadmap.md", "specs/templates/roadmap.md", "specs/decisions.md")
	j.parts = append(j.parts, in)
	j.build = func(files []File, _ bool) ([]File, error) {
		return s.checkPaths(files, func(p string) bool { return p == "specs/roadmap.md" || p == "specs/decisions.md" })
	}
	return nil
}

func (s *Service) prepSpec(j *job, rest string, rows []IndexRow) error {
	first, tail := splitFirst(rest)
	if row, ok := findRow(rows, first); ok && strings.TrimSpace(tail) != "" { // SDD-64: case-sensitive
		return s.prepChange(j, row, tail, rows)
	}
	in, err := s.input(rest)
	if err != nil {
		return err
	}
	j.parts = s.parts("AGENTS.md", "specs/index.md", "specs/roadmap.md")
	j.parts = append(j.parts, s.common()...)
	j.parts = append(j.parts, s.parts("specs/templates/feature.md")...)
	j.parts = append(j.parts, in)
	j.build = s.buildNewSpec
	return nil
}

func (s *Service) prepChange(j *job, row IndexRow, tail string, rows []IndexRow) error {
	in, err := s.input(tail)
	if err != nil {
		return err
	}
	_, fp, err := s.featureParts(row, rows)
	if err != nil {
		return err
	}
	j.id, j.instr = row.ID, instructions["spec-change"]
	j.parts = append(fp, s.parts("specs/templates/change.md", "specs/index.md", "specs/decisions.md")...)
	j.parts = append(j.parts, s.common()...)
	j.parts = append(j.parts, in)
	j.build = func(files []File, _ bool) ([]File, error) { return s.buildChange(row, files) }
	return nil
}

func (s *Service) prepFeature(j *job, rest string, rows []IndexRow) error {
	id, extra := splitFirst(strings.TrimSpace(rest))
	if id == "" || strings.TrimSpace(extra) != "" {
		return fmt.Errorf("/sdd %s takes exactly one feature id. Known ids: %s", j.sub, strings.Join(ids(rows), ", "))
	}
	row, ok := findRow(rows, id)
	if !ok { // SDD-34
		return fmt.Errorf("unknown feature id %q. Known ids: %s", id, strings.Join(ids(rows), ", "))
	}
	f, fp, err := s.featureParts(row, rows)
	if err != nil {
		return err
	}
	j.id = id
	allowFeature := func(p string) bool { return p == row.File || p == indexPath }
	switch j.sub {
	case "spec-validate":
		j.parts = append(fp, s.parts("specs/index.md", "specs/roadmap.md", "specs/decisions.md")...)
		j.parts = append(j.parts, s.common()...)
		j.build = func(files []File, pass bool) ([]File, error) {
			return s.buildStatus(row, files, pass, "ready", allowFeature)
		}
		return nil
	case "implement":
		if err := checkReady(f, id); err != nil { // SDD-55
			return err
		}
	}
	j.parts = append(s.parts("AGENTS.md"), fp...)
	j.parts = append(j.parts, s.common()...)
	src, err := s.sourceParts(f.Modules)
	if err != nil {
		return err
	}
	j.parts = append(j.parts, src...)
	if j.sub == "spec-review" {
		j.build = func(files []File, pass bool) ([]File, error) {
			return s.buildStatus(row, files, true, "implemented", allowFeature)
		}
		return nil
	}
	mods := f.Modules
	j.build = func(files []File, _ bool) ([]File, error) { // SDD-46, SDD-65
		out, err := s.checkPaths(files, func(p string) bool { return underAny(p, mods) })
		if err == nil && len(out) == 0 {
			err = errors.New("the proposal has no files")
		}
		return out, err
	}
	return nil
}

func checkReady(f Feature, id string) error {
	switch f.Status {
	case "ready":
		return nil
	case "implemented":
		return fmt.Errorf("%s is already implemented. Change the spec first with /sdd spec %s <change>, then /sdd spec-validate %s", id, id, id)
	}
	return fmt.Errorf("%s has status %q: only a ready feature can be implemented. Run /sdd spec-validate %s", id, f.Status, id)
}

// featureParts reads a feature file and its depends_on feature files (SDD-41).
func (s *Service) featureParts(row IndexRow, rows []IndexRow) (Feature, []part, error) {
	text, ok := s.read(row.File)
	if !ok {
		return Feature{}, nil, fmt.Errorf("feature file %s (listed in specs/index.md) cannot be read", row.File)
	}
	f, err := parseFeature(text)
	if err != nil {
		return Feature{}, nil, fmt.Errorf("%s: %w", row.File, err)
	}
	parts := []part{{label: row.File, text: text}}
	for _, dep := range f.DependsOn {
		if d, ok := findRow(rows, dep); ok {
			if dt, ok := s.read(d.File); ok {
				parts = append(parts, part{label: d.File, text: dt})
			}
		}
	}
	return f, parts, nil
}

// parts reads existing files as labeled parts; missing files are left out.
func (s *Service) parts(rels ...string) []part {
	var out []part
	for _, r := range rels {
		if t, ok := s.read(r); ok {
			out = append(out, part{label: r, text: t})
		}
	}
	return out
}

// common returns specs/common/*.md in name order.
func (s *Service) common() []part {
	entries, err := s.Files.ReadDir(s.abs("specs/common"))
	if err != nil {
		return nil
	}
	var rels []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			rels = append(rels, "specs/common/"+e.Name())
		}
	}
	sort.Strings(rels)
	return s.parts(rels...)
}

// input reads `<input>`: a file when it is one token naming an existing
// regular file, else free text. Empty input is refused (SDD-33).
func (s *Service) input(rest string) (part, error) {
	in := strings.TrimSpace(rest)
	if in == "" {
		return part{}, errors.New("the input is empty: give free text or a file path")
	}
	if !strings.ContainsFunc(in, unicode.IsSpace) {
		p := in
		if !filepath.IsAbs(p) {
			p = filepath.Join(s.Dir, p)
		}
		if st, err := s.Files.Stat(p); err == nil && st.Mode().IsRegular() {
			b, err := s.Files.Read(p)
			if err != nil {
				return part{}, fmt.Errorf("reading input file %s: %w", in, err)
			}
			return part{label: "input (file: " + in + ")", text: string(b)}, nil
		}
	}
	return part{label: "input", text: in}, nil
}

func splitFirst(s string) (string, string) {
	i := strings.IndexFunc(s, unicode.IsSpace)
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimLeftFunc(s[i:], unicode.IsSpace)
}
