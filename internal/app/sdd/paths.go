package sdd

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// userOwnedName lists the user-owned config file names SDD never writes (ARCH-7, SDD-59).
var userOwnedName = map[string]bool{"models.json": true, "strategies.json": true}

// cleanRel normalizes a proposed path and rejects absolute paths, `..` escapes
// and anything inside `.git/` (SDD-46).
func cleanRel(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	p = filepath.ToSlash(p)
	if strings.HasPrefix(p, "/") || (len(p) > 1 && p[1] == ':') {
		return "", fmt.Errorf("absolute path")
	}
	c := path.Clean(p)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") {
		return "", fmt.Errorf("outside the project dir")
	}
	for _, seg := range strings.Split(c, "/") {
		if seg == ".git" {
			return "", fmt.Errorf("inside .git/")
		}
	}
	return c, nil
}

// resolvesInside reports whether rel, after resolving symlinks in its deepest
// existing part, stays inside the project dir (SDD-46).
func (s *Service) resolvesInside(rel string) error {
	root, err := s.Files.EvalSymlinks(s.Dir)
	if err != nil {
		return fmt.Errorf("resolving the project dir: %w", err)
	}
	abs := filepath.Join(s.Dir, filepath.FromSlash(rel))
	for cur := abs; ; cur = filepath.Dir(cur) {
		if _, err := s.Files.Lstat(cur); err == nil {
			real, err := s.Files.EvalSymlinks(cur)
			if err != nil {
				return fmt.Errorf("resolving symlinks: %w", err)
			}
			if real != root && !strings.HasPrefix(real, root+string(filepath.Separator)) {
				return fmt.Errorf("resolves outside the project dir")
			}
			return nil
		}
		if cur == s.Dir || filepath.Dir(cur) == cur {
			return nil
		}
	}
}

// underAny reports whether rel is one of the folders or inside one.
func underAny(rel string, folders []string) bool {
	for _, f := range folders {
		f = path.Clean(filepath.ToSlash(f))
		if rel == f || strings.HasPrefix(rel, f+"/") {
			return true
		}
	}
	return false
}

// checkPaths rejects the whole proposal when any path is not allowed (SDD-46,
// SDD-47). allow decides per cleaned path. It returns the cleaned files.
func (s *Service) checkPaths(files []File, allow func(string) bool) ([]File, error) {
	var bad []string
	seen := map[string]bool{}
	out := make([]File, 0, len(files))
	for _, f := range files {
		rel, err := cleanRel(f.Path)
		switch {
		case err != nil:
			bad = append(bad, fmt.Sprintf("%s (%v)", f.Path, err))
			continue
		case userOwnedName[path.Base(rel)]:
			bad = append(bad, fmt.Sprintf("%s (models.json and strategies.json are never written)", f.Path))
			continue
		case !allow(rel):
			bad = append(bad, fmt.Sprintf("%s (not an allowed path for this subcommand)", f.Path))
			continue
		case seen[rel]:
			bad = append(bad, fmt.Sprintf("%s (listed twice)", f.Path))
			continue
		}
		if err := s.resolvesInside(rel); err != nil {
			bad = append(bad, fmt.Sprintf("%s (%v)", f.Path, err))
			continue
		}
		seen[rel] = true
		out = append(out, File{Path: rel, Content: f.Content})
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("proposal rejected, nothing written; offending paths:\n  %s", strings.Join(bad, "\n  "))
	}
	return out, nil
}
