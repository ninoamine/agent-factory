package sdd

import (
	"path"
	"regexp"
	"strings"
)

// ignorer applies .gitignore rules without running git (SDD-42, SDD-58).
// It supports the common syntax: comments, `!` negation, trailing `/`,
// leading `/`, `*`, `?`, `**` and character classes. It reads the root
// .gitignore, .git/info/exclude and nested .gitignore files.
type ignorer struct {
	read  func(rel string) (string, bool)
	cache map[string][]rule
}

type rule struct {
	re      *regexp.Regexp
	neg     bool
	dirOnly bool
}

func newIgnorer(read func(rel string) (string, bool)) *ignorer {
	return &ignorer{read: read, cache: map[string][]rule{}}
}

// rulesFor returns the rules found in dir ("" is the project root), in order.
func (g *ignorer) rulesFor(dir string) []rule {
	if r, ok := g.cache[dir]; ok {
		return r
	}
	var rules []rule
	files := []string{path.Join(dir, ".gitignore")}
	if dir == "" {
		files = []string{".git/info/exclude", ".gitignore"}
	}
	for _, f := range files {
		if text, ok := g.read(f); ok {
			for _, line := range strings.Split(text, "\n") {
				if r, ok := parseRule(line); ok {
					rules = append(rules, r)
				}
			}
		}
	}
	g.cache[dir] = rules
	return rules
}

// ignored reports whether rel (slash-separated, relative to the project dir)
// is ignored, directly or through an ignored parent directory.
func (g *ignorer) ignored(rel string, isDir bool) bool {
	segs := strings.Split(rel, "/")
	for i := range segs {
		sub := strings.Join(segs[:i+1], "/")
		dir := i < len(segs)-1 || isDir
		if g.match(sub, dir) {
			return true
		}
	}
	return false
}

func (g *ignorer) match(rel string, isDir bool) bool {
	result := false
	// Rules from the root down to the parent directory; later rules win.
	dirs := []string{""}
	segs := strings.Split(rel, "/")
	for i := 1; i < len(segs); i++ {
		dirs = append(dirs, strings.Join(segs[:i], "/"))
	}
	for _, d := range dirs {
		sub := rel
		if d != "" {
			sub = strings.TrimPrefix(rel, d+"/")
		}
		for _, r := range g.rulesFor(d) {
			if r.dirOnly && !isDir {
				continue
			}
			if r.re.MatchString(sub) {
				result = !r.neg
			}
		}
	}
	return result
}

func parseRule(line string) (rule, bool) {
	line = strings.TrimRight(strings.TrimSuffix(line, "\r"), " ")
	if line == "" || strings.HasPrefix(line, "#") {
		return rule{}, false
	}
	var r rule
	if strings.HasPrefix(line, "!") {
		r.neg, line = true, line[1:]
	}
	if strings.HasSuffix(line, "/") {
		r.dirOnly, line = true, strings.TrimSuffix(line, "/")
	}
	anchored := strings.Contains(line, "/")
	line = strings.TrimPrefix(line, "/")
	if line == "" {
		return rule{}, false
	}
	body := globToRegexp(line)
	expr := "^" + body + "$"
	if !anchored {
		expr = "^(?:.*/)?" + body + "$"
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return rule{}, false
	}
	r.re = re
	return r, true
}

func globToRegexp(g string) string {
	var b strings.Builder
	rs := []rune(g)
	for i := 0; i < len(rs); i++ {
		switch c := rs[i]; c {
		case '*':
			if i+1 < len(rs) && rs[i+1] == '*' {
				switch {
				case i+2 < len(rs) && rs[i+2] == '/':
					b.WriteString("(?:.*/)?")
					i += 2
				case i+2 >= len(rs):
					b.WriteString(".*")
					i++
				default:
					b.WriteString("[^/]*")
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '[':
			j := i + 1
			for j < len(rs) && rs[j] != ']' {
				j++
			}
			if j >= len(rs) {
				b.WriteString(`\[`)
				continue
			}
			cls := string(rs[i+1 : j])
			if strings.HasPrefix(cls, "!") {
				cls = "^" + cls[1:]
			}
			b.WriteString("[" + cls + "]")
			i = j
		case '\\':
			if i+1 < len(rs) {
				i++
				b.WriteString(regexp.QuoteMeta(string(rs[i])))
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}
