package sdd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/history"
)

const validAnswers = "valid answers: yes | no | redo [feedback] | edit"

// collector forwards output to the screen and keeps the model text.
type collector struct {
	out execution.Output
	mu  sync.Mutex
	buf strings.Builder
}

func (c *collector) Chunk(t string)  { c.add(t); c.out.Chunk(t) }
func (c *collector) Final(t string)  { c.add(t); c.out.Final(t) }
func (c *collector) Notice(t string) { c.out.Notice(t) }
func (c *collector) Error(t string)  { c.out.Error(t) }
func (c *collector) add(t string) {
	c.mu.Lock()
	c.buf.WriteString(t)
	c.mu.Unlock()
}
func (c *collector) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// prompt assembles instruction and labeled context, and enforces the size cap
// before any model call (SDD-43, SDD-62).
func (j *job) prompt() (string, error) {
	parts := append([]part(nil), j.parts...)
	if j.prev != "" {
		parts = append(parts, part{label: "previous proposal", text: j.prev}, part{label: "feedback", text: j.feedback})
	}
	total := len(j.instr)
	for _, p := range parts {
		total += len(p.label) + 12 + max(p.size, len(p.text))
	}
	if total > SizeCap {
		sort.SliceStable(parts, func(a, b int) bool {
			return max(parts[a].size, len(parts[a].text)) > max(parts[b].size, len(parts[b].text))
		})
		var names []string
		for i, p := range parts {
			if i == 3 {
				break
			}
			names = append(names, fmt.Sprintf("%s (%d KB)", p.label, (max(p.size, len(p.text))+1023)/1024))
		}
		return "", fmt.Errorf("the context is %d KB, over the %d KB limit; nothing was sent. Largest files: %s",
			(total+1023)/1024, SizeCap/1024, strings.Join(names, ", "))
	}
	var b strings.Builder
	b.WriteString(j.instr)
	for _, p := range parts {
		fmt.Fprintf(&b, "\n\n=== %s ===\n%s", p.label, p.text)
	}
	return b.String(), nil
}

// execute runs the model once (SDD-39) and turns its answer into a verdict
// and/or a proposal waiting for approval.
func (s *Service) execute(ctx context.Context, j *job, out execution.Output) (Reply, error) {
	prompt, err := j.prompt()
	if err != nil {
		return Reply{}, err
	}
	cfg, release := s.Config()
	defer release()
	d := j.decision
	d.Prompt = prompt
	col := &collector{out: out}
	out.Notice(fmt.Sprintf("%s (/sdd %s):", d.Model, j.sub))
	rec := s.Runner.RunSDD(ctx, cfg, d, j.sub, col)
	if rec.Status != execution.StatusSuccess {
		return Reply{Lines: []string{"nothing was written"}}, nil
	}
	return s.afterRun(j, col.text())
}

func (s *Service) afterRun(j *job, text string) (Reply, error) {
	pass, found := readVerdict(text)
	note := ""
	if j.sub == "spec-validate" || j.sub == "spec-review" || j.sub == "review" {
		note = "verdict: " + map[bool]string{true: "pass", false: "fail"}[pass]
		if !found {
			note += " (no VERDICT marker found; counted as fail)"
		}
	}
	if j.sub == "review" {
		return Reply{Lines: []string{note, "review writes nothing"}}, nil
	}
	if j.sub == "spec-review" && !pass {
		return Reply{Lines: []string{note, "the gaps are shown above; nothing was written"}}, nil
	}
	j.pass = pass

	files, deletes, perr := parseProposal(text)
	if len(deletes) > 0 { // SDD-47
		return Reply{}, fmt.Errorf("proposal rejected, nothing written: it asks to delete files: %s", strings.Join(deletes, ", "))
	}
	statusOnly := j.sub == "spec-review" || (j.sub == "spec-validate" && pass)
	if perr != nil && !statusOnly {
		if j.sub == "spec-validate" { // fail without a corrected spec
			return Reply{Lines: []string{note, "no corrected spec was proposed; the problems are shown above; nothing was written"}}, nil
		}
		return Reply{}, fmt.Errorf("could not read the proposal: %v. The raw output is shown above; nothing was written", perr) // SDD-45
	}
	final, err := j.build(files, pass)
	if err != nil {
		return Reply{}, err
	}
	r, err := s.present(j, final, text)
	if note != "" {
		r.Lines = append([]string{note}, r.Lines...)
	}
	return r, err
}

// present shows each changed file with a diff and waits for approval (SDD-48).
func (s *Service) present(j *job, files []File, raw string) (Reply, error) {
	var changed []File
	var lines []string
	for _, f := range files {
		old, exists := s.read(f.Path)
		if exists && old == f.Content {
			continue
		}
		changed = append(changed, f)
		kind := "changed"
		if !exists {
			kind = "new"
		}
		lines = append(lines, fmt.Sprintf("%s file: %s", kind, f.Path))
		lines = append(lines, unifiedDiff(old, f.Content)...)
	}
	if len(changed) == 0 {
		return Reply{Lines: []string{"the proposal changes nothing; nothing was written"}}, nil
	}
	lines = append(lines, "Write these files? "+validAnswers)
	s.mu.Lock()
	s.pend = &pending{j: j, files: changed, raw: raw}
	s.mu.Unlock()
	return Reply{Lines: []string{strings.Join(lines, "\n")}, Waiting: true}, nil
}

// Answer handles the approval prompt (SDD-49). Ctrl+C at the prompt is "no".
func (s *Service) Answer(ctx context.Context, line string, out execution.Output) (Reply, error) {
	s.mu.Lock()
	p := s.pend
	s.mu.Unlock()
	if p == nil {
		return Reply{}, errors.New("no SDD proposal is waiting for approval")
	}
	word, rest := splitFirst(strings.TrimSpace(line))
	switch strings.ToLower(word) {
	case "yes", "y":
		return s.settle(s.write(ctx, p))
	case "no", "n":
		return s.settle(Reply{Lines: []string{"discarded; nothing was written"}}, nil)
	case "redo":
		s.mu.Lock()
		s.pend = nil
		s.mu.Unlock()
		p.j.prev, p.j.feedback = p.raw, rest
		return s.settle(s.execute(ctx, p.j, out))
	case "edit":
		return s.edit(ctx, p)
	}
	return Reply{Lines: []string{validAnswers}, Waiting: true}, nil
}

func (s *Service) edit(ctx context.Context, p *pending) (Reply, error) {
	command := s.Getenv("VISUAL")
	if strings.TrimSpace(command) == "" {
		command = s.Getenv("EDITOR")
	}
	wait := func(lines ...string) Reply { return Reply{Lines: lines, Waiting: true} }
	if strings.TrimSpace(command) == "" {
		return wait("no editor is set: set $VISUAL or $EDITOR to use edit. " + validAnswers), nil // SDD-50
	}
	text, err := s.Editor.Edit(ctx, command, formatProposal(p.files))
	if err != nil {
		return wait("the editor failed: "+err.Error(), "the proposal is unchanged. "+validAnswers), nil
	}
	if strings.TrimSpace(text) == "" {
		return wait("the edited file is empty; the proposal is unchanged. " + validAnswers), nil
	}
	files, deletes, perr := parseProposal(text)
	if perr == nil && len(deletes) > 0 {
		perr = fmt.Errorf("it asks to delete files: %s", strings.Join(deletes, ", "))
	}
	var final []File
	if perr == nil {
		final, perr = p.j.build(files, p.j.pass) // SDD-46 re-check
	}
	if perr != nil {
		return Reply{Lines: []string{"the edited proposal was rejected and the previous one is kept. " + validAnswers}, Waiting: true},
			fmt.Errorf("edited proposal rejected: %w", perr)
	}
	r, err := s.present(p.j, final, formatProposal(final))
	return s.settle(r, err) // an edit that removes every change ends the run
}

// write saves the approved files atomically and reports what was and was not written (SDD-51).
func (s *Service) write(ctx context.Context, p *pending) (Reply, error) {
	var written, failed []string
	var firstErr error
	for _, f := range p.files {
		err := s.resolvesInside(f.Path)
		if err == nil {
			err = s.Files.WriteAtomic(ctx, s.abs(f.Path), []byte(f.Content))
		}
		if err != nil {
			failed = append(failed, f.Path)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		written = append(written, f.Path)
	}
	if firstErr != nil {
		return Reply{Lines: []string{"written: " + strings.Join(written, ", ")}},
			fmt.Errorf("writing failed: %v. Written: %s. Not written: %s", firstErr, orNone(written), strings.Join(failed, ", "))
	}
	return Reply{Lines: []string{"written: " + strings.Join(written, ", ")}}, nil
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, ", ")
}

// init lists what /sdd init would create and skip, then asks (SDD-35, SDD-36).
func (s *Service) init(ctx context.Context) (Reply, error) {
	var create, skip []initFile
	for _, f := range initFiles {
		if s.exists(f.Path) {
			skip = append(skip, f)
		} else {
			create = append(create, f)
		}
	}
	var dirs []string
	for _, d := range initDirs {
		if !s.exists(d) {
			dirs = append(dirs, d)
		}
	}
	var b strings.Builder
	b.WriteString("/sdd init will create:\n")
	for _, f := range create {
		b.WriteString("  " + f.Path + "\n")
	}
	for _, d := range dirs {
		b.WriteString("  " + d + "/ (empty directory)\n")
	}
	if len(skip) > 0 {
		b.WriteString("and will skip these existing files (never overwritten):\n")
		for _, f := range skip {
			b.WriteString("  " + f.Path + "\n")
		}
	}
	if len(create) == 0 && len(dirs) == 0 {
		return Reply{Lines: []string{"nothing to create: the SDD files already exist"}}, nil
	}
	b.WriteString("Create them? (yes/no)")
	return Reply{Confirm: &history.Confirmation{
		Prompt: b.String(),
		Run: func(ctx context.Context) ([]string, error) {
			return s.createInit(ctx, create, dirs)
		},
	}}, nil
}

func (s *Service) createInit(ctx context.Context, create []initFile, dirs []string) ([]string, error) {
	var done []string
	for _, d := range dirs {
		if err := s.resolvesInside(d); err != nil {
			return done, fmt.Errorf("creating %s: %w", d, err)
		}
		if err := s.Files.MkdirAll(s.abs(d)); err != nil {
			return done, fmt.Errorf("creating %s: %w", d, err)
		}
		done = append(done, "created "+d+"/")
	}
	for _, f := range create {
		if s.exists(f.Path) { // never overwrite (SDD-36)
			done = append(done, "skipped "+f.Path+" (it appeared meanwhile)")
			continue
		}
		if err := s.resolvesInside(f.Path); err != nil {
			return done, fmt.Errorf("creating %s: %w", f.Path, err)
		}
		if err := s.Files.WriteAtomic(ctx, s.abs(filepath.FromSlash(f.Path)), []byte(f.Content)); err != nil {
			return done, fmt.Errorf("creating %s: %w", f.Path, err)
		}
		done = append(done, "created "+f.Path)
	}
	return done, nil
}
