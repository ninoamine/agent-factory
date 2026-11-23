package sdd

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const decisionsPath = "specs/decisions.md"

var featurePathRe = regexp.MustCompile(`^specs/features/[^/]+\.md$`)

// find returns the content of rel among cleaned files, falling back to disk.
func (s *Service) find(files []File, rel string) (string, bool) {
	for _, f := range files {
		if f.Path == rel {
			return f.Content, true
		}
	}
	return s.read(rel)
}

// withoutPaths returns files minus the given paths.
func withoutPaths(files []File, rels ...string) []File {
	var out []File
	for _, f := range files {
		drop := false
		for _, r := range rels {
			drop = drop || f.Path == r
		}
		if !drop {
			out = append(out, f)
		}
	}
	return out
}

// setStatuses sets the feature status in the feature file and the index row.
// The tool does this itself; model text is never trusted for status (SDD-52).
// The index must keep every other row exactly as on disk, and the target row
// must point at row.File (SDD-52, SDD-53).
func (s *Service) setStatuses(files []File, row IndexRow, status string) ([]File, error) {
	feat, ok := s.find(files, row.File)
	if !ok {
		return nil, fmt.Errorf("feature file %s cannot be read", row.File)
	}
	feat, err := setFeatureStatus(feat, status)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", row.File, err)
	}
	idx, ok := s.find(files, indexPath)
	if !ok {
		return nil, errors.New("specs/index.md cannot be read")
	}
	if err := s.checkIndexRows(idx, row); err != nil {
		return nil, err
	}
	idx, ok = setIndexStatus(idx, row.ID, status)
	if !ok {
		return nil, fmt.Errorf("specs/index.md has no row for %s", row.ID)
	}
	out := withoutPaths(files, row.File, indexPath)
	return append(out, File{row.File, feat}, File{indexPath, idx}), nil
}

// indexCells returns the cells of every feature row of an index, keyed by id.
// A repeated id is an error: findRow and setIndexStatus would use the first
// row only, so a second one could hide a changed status (SDD-52).
func indexCells(text string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, line := range strings.Split(text, "\n") {
		c := cells(line)
		if len(c) < 3 || c[0] == "ID" || strings.HasPrefix(c[0], "---") || c[0] == "" {
			continue
		}
		if _, dup := out[c[0]]; dup {
			return nil, fmt.Errorf("proposal rejected: specs/index.md lists %s more than once", c[0])
		}
		out[c[0]] = c
	}
	return out, nil
}

// indexFile is the project-relative file of a row's cells, as parseIndex reads it.
func indexFile(c []string) string { return "specs/" + strings.TrimPrefix(c[1], "specs/") }

// checkIndexRows rejects a proposed index that changes, adds or removes any
// row other than the target, in any column, or points the target at another
// file (SDD-52, SDD-53). The target row keeps its file but its other cells may
// change, since a spec change can touch depends_on or modules (SDD-64); the
// tool sets its status afterwards.
func (s *Service) checkIndexRows(proposed string, target IndexRow) error {
	current, _ := s.read(indexPath)
	want, _ := indexCells(current) // a damaged index on disk is not the model's fault
	got, err := indexCells(proposed)
	if err != nil {
		return err
	}
	for id, w := range want {
		g, ok := got[id]
		switch {
		case !ok:
			return fmt.Errorf("proposal rejected: it removes the %s row from specs/index.md", id)
		case id == target.ID && indexFile(g) != indexFile(w):
			return fmt.Errorf("proposal rejected: the %s row must keep its file %s", id, indexFile(w))
		case id != target.ID && !slices.Equal(g, w):
			return fmt.Errorf("proposal rejected: it changes the %s row of specs/index.md; statuses are set only by the tool (SDD-52)", id)
		}
	}
	for id, g := range got {
		if _, old := want[id]; old {
			continue
		}
		if id != target.ID || indexFile(g) != target.File {
			return fmt.Errorf("proposal rejected: specs/index.md has an unexpected new row %s (%s)", id, g[1])
		}
	}
	return nil
}

// buildStatus is used by spec-validate and spec-review (SDD-54). With pass,
// only the status changes, starting from the files on disk: the model's text
// is never written as validated. With fail (validate only) the model's
// corrected spec is required and stays draft.
func (s *Service) buildStatus(row IndexRow, files []File, pass bool, passStatus string, allow func(string) bool) ([]File, error) {
	if pass {
		return s.setStatuses(nil, row, passStatus)
	}
	cleaned, err := s.checkPaths(files, allow)
	if err != nil {
		return nil, err
	}
	text, ok := s.find(cleaned, row.File)
	if !ok || len(cleaned) == 0 {
		return nil, errors.New("no corrected spec was proposed")
	}
	if f, err := parseFeature(text); err != nil || f.ID != row.ID {
		return nil, fmt.Errorf("the corrected spec must keep front matter id %s", row.ID)
	}
	return s.setStatuses(cleaned, row, "draft")
}

// buildNewSpec checks a new-spec proposal (SDD-46, SDD-53).
func (s *Service) buildNewSpec(files []File, _ bool) ([]File, error) {
	cleaned, err := s.checkPaths(files, func(p string) bool { return p == indexPath || featurePathRe.MatchString(p) })
	if err != nil {
		return nil, err
	}
	var feat *File
	hasIndex := false
	for i, f := range cleaned {
		switch {
		case f.Path == indexPath:
			hasIndex = true
		case feat != nil:
			return nil, errors.New("proposal rejected: a new spec must contain exactly one feature file")
		default:
			feat = &cleaned[i]
		}
	}
	if feat == nil || !hasIndex {
		return nil, errors.New("proposal rejected: a new spec needs one specs/features/<name>.md and specs/index.md")
	}
	if s.exists(feat.Path) {
		return nil, fmt.Errorf("proposal rejected: %s already exists", feat.Path)
	}
	f, err := parseFeature(feat.Content)
	if err != nil {
		return nil, fmt.Errorf("proposal rejected: %s: %w", feat.Path, err)
	}
	current, _ := s.read(indexPath)
	if _, dup := findRow(parseIndex(current), f.ID); dup {
		return nil, fmt.Errorf("proposal rejected: feature id %s is already in specs/index.md", f.ID)
	}
	return s.setStatuses(cleaned, IndexRow{ID: f.ID, File: feat.Path}, "draft")
}

// buildChange checks a spec change (SDD-64): same id, decisions append-only,
// status reset to draft in both places.
func (s *Service) buildChange(row IndexRow, files []File) ([]File, error) {
	cleaned, err := s.checkPaths(files, func(p string) bool { return p == row.File || p == indexPath || p == decisionsPath })
	if err != nil {
		return nil, err
	}
	text, ok := s.find(cleaned, row.File)
	if !ok || !hasPath(cleaned, row.File) {
		return nil, fmt.Errorf("proposal rejected: it must contain %s", row.File)
	}
	if f, err := parseFeature(text); err != nil || f.ID != row.ID {
		return nil, fmt.Errorf("proposal rejected: the feature id must stay %s", row.ID)
	}
	if old, ok := s.read(row.File); ok {
		if missing := missingRuleIDs(old, text); len(missing) > 0 {
			return nil, fmt.Errorf("proposal rejected: existing rule IDs must stay (never renumber, remove or reuse): missing %s", strings.Join(missing, ", "))
		}
	}
	if hasPath(cleaned, decisionsPath) {
		old, _ := s.read(decisionsPath)
		nu, _ := s.find(cleaned, decisionsPath)
		if !strings.HasPrefix(nu, old) {
			return nil, errors.New("proposal rejected: specs/decisions.md may only get new lines appended")
		}
	}
	return s.setStatuses(cleaned, row, "draft")
}

var ruleIDRe = regexp.MustCompile(`(?m)^\s*[-*]\s+([A-Z][A-Z0-9]*-\d+)\b`)

// missingRuleIDs lists rule IDs (lines like `- AUTH-3 ...`) of the old spec that
// no longer appear as rules in the new one (SDD-64). A rule that is dropped is
// kept in place with its ID, for example marked as removed in its text.
func missingRuleIDs(oldText, newText string) []string {
	have := map[string]bool{}
	for _, m := range ruleIDRe.FindAllStringSubmatch(newText, -1) {
		have[m[1]] = true
	}
	var missing []string
	seen := map[string]bool{}
	for _, m := range ruleIDRe.FindAllStringSubmatch(oldText, -1) {
		if !have[m[1]] && !seen[m[1]] {
			seen[m[1]] = true
			missing = append(missing, m[1])
		}
	}
	return missing
}

func hasPath(files []File, rel string) bool {
	for _, f := range files {
		if f.Path == rel {
			return true
		}
	}
	return false
}
