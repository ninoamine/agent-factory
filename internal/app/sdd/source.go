package sdd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

const binaryProbe = 8000

func (s *Service) newIgnorer() *ignorer {
	return newIgnorer(s.read)
}

// isBinary reports whether the start of a file contains a NUL byte.
func (s *Service) isBinary(rel string) bool {
	head, err := s.Files.Head(s.abs(rel), binaryProbe)
	return err != nil || bytes.IndexByte(head, 0) >= 0
}

// sourceParts reads the files under the feature's modules folders (SDD-42):
// no .git/, no binary files, no git-ignored files, nothing outside the project
// dir. A file over the size cap is kept as a size-only part so the refusal can
// name it (SDD-43, SDD-61).
func (s *Service) sourceParts(modules []string) ([]part, error) {
	ign := s.newIgnorer()
	var out []part
	seen := map[string]bool{}
	for _, m := range modules {
		rel, err := cleanRel(m)
		if err != nil {
			return nil, fmt.Errorf("module %q is not a valid project path: %w", m, err)
		}
		if err := s.resolvesInside(rel); err != nil {
			return nil, fmt.Errorf("module %q: %w", m, err)
		}
		if ign.ignored(rel, true) {
			continue
		}
		if err := s.walk(rel, ign, seen, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) walk(rel string, ign *ignorer, seen map[string]bool, out *[]part) error {
	st, err := s.Files.Lstat(s.abs(rel))
	if err != nil {
		return nil // a module folder that does not exist yet has no files
	}
	switch {
	case st.Mode()&fs.ModeSymlink != 0:
		return nil
	case st.IsDir():
		entries, err := s.Files.ReadDir(s.abs(rel))
		if err != nil {
			return fmt.Errorf("reading %s: %w", rel, err)
		}
		for _, e := range entries {
			if e.Name() == ".git" {
				continue
			}
			child := path.Join(rel, e.Name())
			if ign.ignored(child, e.IsDir()) {
				continue
			}
			if err := s.walk(child, ign, seen, out); err != nil {
				return err
			}
		}
	case st.Mode().IsRegular():
		if seen[rel] || s.isBinary(rel) {
			return nil
		}
		seen[rel] = true
		if st.Size() > SizeCap {
			*out = append(*out, part{label: rel, size: int(st.Size())})
			return nil
		}
		b, err := s.Files.Read(s.abs(rel))
		if err != nil {
			return fmt.Errorf("reading %s: %w", rel, err)
		}
		*out = append(*out, part{label: rel, text: string(b)})
	}
	return nil
}

var diffHeader = regexp.MustCompile(`(?m)^diff --git a/.+ b/(.+)$`)

// prepReview builds the review context (SDD-56): the uncommitted diff from
// `git diff HEAD` plus untracked, non-ignored files. No changes means no model
// call, so j.parts stays empty.
func (s *Service) prepReview(ctx context.Context, j *job, rows []IndexRow) (Reply, error) {
	diff, err := s.Git.Diff(ctx, s.Dir)
	if err != nil {
		if err == ErrNotRepo || strings.Contains(err.Error(), ErrNotRepo.Error()) {
			return Reply{}, fmt.Errorf("/sdd review needs a git repository: %s is not inside one", s.Dir)
		}
		if errors.Is(err, ErrNoCommits) {
			return Reply{}, errors.New("/sdd review needs at least one commit: this repository has none yet. Make a first commit, then run it again")
		}
		return Reply{}, fmt.Errorf("running git diff HEAD: %w", err)
	}
	untracked, err := s.Git.Untracked(ctx, s.Dir)
	if err != nil {
		return Reply{}, fmt.Errorf("listing untracked files: %w", err)
	}
	if strings.TrimSpace(diff) == "" && len(untracked) == 0 {
		return Reply{Lines: []string{"there are no uncommitted changes to review; no model was called"}}, nil
	}

	changed := map[string]bool{}
	for _, m := range diffHeader.FindAllStringSubmatch(diff, -1) {
		changed[m[1]] = true
	}
	j.parts = s.parts("AGENTS.md", "specs/common/conventions.md")
	if strings.TrimSpace(diff) != "" {
		j.parts = append(j.parts, part{label: "git diff HEAD", text: diff})
	}
	ign := s.newIgnorer()
	for _, u := range untracked {
		rel, err := cleanRel(u)
		if err != nil || s.resolvesInside(rel) != nil || ign.ignored(rel, false) {
			continue
		}
		st, err := s.Files.Lstat(s.abs(rel))
		if err != nil || !st.Mode().IsRegular() || s.isBinary(rel) {
			continue
		}
		changed[rel] = true
		if st.Size() > SizeCap {
			j.parts = append(j.parts, part{label: "untracked: " + rel, size: int(st.Size())})
			continue
		}
		if b, err := s.Files.Read(s.abs(rel)); err == nil {
			j.parts = append(j.parts, part{label: "untracked: " + rel, text: string(b)})
		}
	}
	var paths []string
	for p := range changed {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, r := range rows {
		text, ok := s.read(r.File)
		if !ok {
			continue
		}
		f, err := parseFeature(text)
		if err != nil {
			continue
		}
		for _, p := range paths {
			if underAny(p, f.Modules) {
				j.parts = append(j.parts, part{label: r.File, text: text})
				break
			}
		}
	}
	return Reply{}, nil
}
