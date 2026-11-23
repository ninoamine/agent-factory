// Package gitcli runs the only external commands SDD may run (ARCH-12):
// `git diff HEAD` and the listing of untracked files. No shell is used.
package gitcli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ninoamine/agent-factory/internal/app/sdd"
)

// ErrNotRepo means the project dir is not inside a git repository (SDD-56).
var ErrNotRepo = sdd.ErrNotRepo

// ErrNoCommits means the repository has no commit, so HEAD does not exist (SDD-56).
var ErrNoCommits = sdd.ErrNoCommits

// Git implements the sdd.Git interface.
type Git struct{}

// Diff returns the output of `git diff HEAD` run in dir.
func (Git) Diff(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "diff", "HEAD")
	return string(out), err
}

// Untracked lists untracked files that are not ignored, relative to dir.
func (Git) Untracked(ctx context.Context, dir string) ([]string, error) {
	out, err := run(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

func run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "not a git repository") || strings.Contains(msg, "Not a git repository") {
			return nil, ErrNotRepo
		}
		if strings.Contains(msg, "ambiguous argument 'HEAD'") || strings.Contains(msg, "bad revision 'HEAD'") || strings.Contains(msg, "unknown revision") {
			return nil, ErrNoCommits
		}
		return nil, fmt.Errorf("running git %s: %w: %s", args[0], err, msg)
	}
	return stdout.Bytes(), nil
}
