package sdd

import (
	"fmt"
	"strings"
)

const diffContext = 3

// unifiedDiff renders a line diff of old against new with a few context lines.
// It is for display only (SDD-48); proposals never contain diffs (SDD-61).
func unifiedDiff(oldText, newText string) []string {
	a, b := splitLines(oldText), splitLines(newText)
	ops := diffOps(a, b)
	keep := make([]bool, len(ops))
	for i, op := range ops {
		if op.kind == ' ' {
			continue
		}
		for j := max(0, i-diffContext); j <= min(len(ops)-1, i+diffContext); j++ {
			keep[j] = true
		}
	}
	var out []string
	skipped := false
	for i, op := range ops {
		if !keep[i] {
			skipped = true
			continue
		}
		if skipped && len(out) > 0 {
			out = append(out, "  ...")
		}
		skipped = false
		out = append(out, fmt.Sprintf("%c %s", op.kind, op.text))
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

type diffOp struct {
	kind byte // ' ', '+', '-'
	text string
}

// diffOps is a longest-common-subsequence diff. Very large inputs fall back to
// "everything removed, everything added" to bound the work.
func diffOps(a, b []string) []diffOp {
	// Trim the common prefix and suffix first.
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	var ops []diffOp
	for _, l := range a[:p] {
		ops = append(ops, diffOp{' ', l})
	}
	ma, mb := a[p:len(a)-s], b[p:len(b)-s]
	ops = append(ops, lcsOps(ma, mb)...)
	for _, l := range a[len(a)-s:] {
		ops = append(ops, diffOp{' ', l})
	}
	return ops
}

func lcsOps(a, b []string) []diffOp {
	if len(a)*len(b) > 4_000_000 {
		var ops []diffOp
		for _, l := range a {
			ops = append(ops, diffOp{'-', l})
		}
		for _, l := range b {
			ops = append(ops, diffOp{'+', l})
		}
		return ops
	}
	n, m := len(a), len(b)
	t := make([][]int, n+1)
	for i := range t {
		t[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				t[i][j] = t[i+1][j+1] + 1
			} else {
				t[i][j] = max(t[i+1][j], t[i][j+1])
			}
		}
	}
	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i]})
			i++
			j++
		case t[i+1][j] >= t[i][j+1]:
			ops = append(ops, diffOp{'-', a[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', b[j]})
	}
	return ops
}
