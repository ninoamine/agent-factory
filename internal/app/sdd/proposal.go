package sdd

import (
	"fmt"
	"regexp"
	"strings"
)

// File is one proposed file with its full content (SDD-61).
type File struct {
	Path    string
	Content string
}

var (
	fileRe   = regexp.MustCompile(`^=== FILE: (.+?) ===\s*$`)
	endRe    = regexp.MustCompile(`^=== END FILE ===\s*$`)
	deleteRe = regexp.MustCompile(`^=== DELETE: (.+?) ===\s*$`)
	verdict  = regexp.MustCompile(`(?i)^\s*VERDICT:\s*(pass|fail)\s*$`)
)

// parseProposal reads the fixed proposal format (SDD-45): one block per file,
//
//	=== FILE: path ===
//	<full content>
//	=== END FILE ===
//
// Text outside the blocks is ignored. A DELETE block is returned so the caller
// can reject the proposal (SDD-47). Output with no block, an unclosed block or
// a diff instead of full content is unparsable (SDD-61).
func parseProposal(text string) (files []File, deletes []string, err error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		if m := deleteRe.FindStringSubmatch(lines[i]); m != nil {
			deletes = append(deletes, strings.TrimSpace(m[1]))
			continue
		}
		m := fileRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		path := strings.TrimSpace(m[1])
		var body []string
		closed := false
		for i++; i < len(lines); i++ {
			if endRe.MatchString(lines[i]) {
				closed = true
				break
			}
			if fileRe.MatchString(lines[i]) {
				return nil, nil, fmt.Errorf("file block %q is not closed with %q", path, "=== END FILE ===")
			}
			body = append(body, lines[i])
		}
		if !closed {
			return nil, nil, fmt.Errorf("file block %q is not closed with %q", path, "=== END FILE ===")
		}
		content := ""
		if len(body) > 0 {
			content = strings.Join(body, "\n") + "\n"
		}
		if looksLikeDiff(body) {
			return nil, nil, fmt.Errorf("file block %q contains a diff; proposals must contain full file content", path)
		}
		files = append(files, File{Path: path, Content: content})
	}
	if len(files) == 0 && len(deletes) == 0 {
		return nil, nil, fmt.Errorf("no %q block found", "=== FILE: <path> ===")
	}
	return files, deletes, nil
}

func looksLikeDiff(body []string) bool {
	hunk, header := false, false
	for i, l := range body {
		if strings.HasPrefix(l, "diff --git ") {
			return true
		}
		if strings.HasPrefix(l, "@@ ") && strings.Contains(l[3:], " @@") {
			hunk = true
		}
		if strings.HasPrefix(l, "--- ") && i+1 < len(body) && strings.HasPrefix(body[i+1], "+++ ") {
			header = true
		}
	}
	return hunk && header
}

// formatProposal renders files in the proposal format, for `edit` (SDD-49).
func formatProposal(files []File) string {
	var b strings.Builder
	for _, f := range files {
		fmt.Fprintf(&b, "=== FILE: %s ===\n%s", f.Path, f.Content)
		if f.Content != "" && !strings.HasSuffix(f.Content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("=== END FILE ===\n\n")
	}
	return b.String()
}

// readVerdict reads the marker on the last non-empty line; a missing or
// unreadable marker is a fail (SDD-54).
func readVerdict(text string) (pass bool, found bool) {
	lines := strings.Split(strings.TrimRight(text, " \t\r\n"), "\n")
	m := verdict.FindStringSubmatch(lines[len(lines)-1])
	if m == nil {
		return false, false
	}
	return strings.EqualFold(m[1], "pass"), true
}
