package sdd

import (
	"fmt"
	"regexp"
	"strings"
)

// IndexRow is one feature row of specs/index.md.
type IndexRow struct {
	ID     string
	File   string // path relative to the project dir, for example specs/features/auth.md
	Status string
}

// Feature is the front matter of a feature file.
type Feature struct {
	ID        string
	Status    string
	Version   string
	DependsOn []string
	Modules   []string
}

const indexPath = "specs/index.md"

// parseIndex reads the feature table of specs/index.md. Rows are
// `| ID | File | Status | Version | depends_on | Modules |`.
func parseIndex(text string) []IndexRow {
	var rows []IndexRow
	for _, line := range strings.Split(text, "\n") {
		c := cells(line)
		if len(c) < 3 || c[0] == "ID" || strings.HasPrefix(c[0], "---") || c[0] == "" {
			continue
		}
		rows = append(rows, IndexRow{ID: c[0], File: "specs/" + strings.TrimPrefix(c[1], "specs/"), Status: c[2]})
	}
	return rows
}

// cells splits a markdown table row into trimmed cells; nil when not a row.
func cells(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return nil
	}
	parts := strings.Split(strings.Trim(line, "|"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func findRow(rows []IndexRow, id string) (IndexRow, bool) {
	for _, r := range rows {
		if r.ID == id {
			return r, true
		}
	}
	return IndexRow{}, false
}

func ids(rows []IndexRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

// setIndexStatus rewrites the status cell of one feature row (SDD-52).
// It reports false when the row is missing.
func setIndexStatus(content, id, status string) (string, bool) {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		c := cells(line)
		if len(c) < 3 || c[0] != id {
			continue
		}
		c[2] = status
		lines[i] = "| " + strings.Join(c, " | ") + " |"
		return strings.Join(lines, "\n"), true
	}
	return content, false
}

var frontRe = regexp.MustCompile(`(?s)\A---\r?\n(.*?)\r?\n---[ \t]*(\r?\n|\z)`)

// parseFeature reads and checks the front matter of a feature file (SDD-53).
func parseFeature(text string) (Feature, error) {
	m := frontRe.FindStringSubmatch(text)
	if m == nil {
		return Feature{}, fmt.Errorf("missing front matter (--- block at the top of the file)")
	}
	kv := map[string]string{}
	for _, line := range strings.Split(m[1], "\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok {
			kv[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	f := Feature{ID: kv["id"], Status: kv["status"], Version: kv["version"],
		DependsOn: list(kv["depends_on"]), Modules: list(kv["modules"])}
	if f.ID == "" {
		return Feature{}, fmt.Errorf("front matter has no id")
	}
	if strings.ContainsAny(f.ID, " \t|") {
		return Feature{}, fmt.Errorf("front matter id %q is not valid", f.ID)
	}
	return f, nil
}

func list(v string) []string {
	v = strings.Trim(strings.TrimSpace(v), "[]")
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" && p != "none" {
			out = append(out, p)
		}
	}
	return out
}

// setFeatureStatus rewrites (or inserts) the status line in the front matter (SDD-52).
func setFeatureStatus(text, status string) (string, error) {
	m := frontRe.FindStringSubmatchIndex(text)
	if m == nil {
		return text, fmt.Errorf("missing front matter")
	}
	block := text[m[2]:m[3]]
	lines := strings.Split(block, "\n")
	done := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "status:") {
			lines[i] = "status: " + status
			done = true
		}
	}
	if !done {
		lines = append([]string{lines[0]}, append([]string{"status: " + status}, lines[1:]...)...)
	}
	return text[:m[2]] + strings.Join(lines, "\n") + text[m[3]:], nil
}
