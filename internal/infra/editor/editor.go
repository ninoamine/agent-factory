// Package editor opens a proposal in the user's editor (SDD-50). The editor
// command comes from $VISUAL or $EDITOR and is started without a shell.
package editor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Editor implements the sdd.Editor interface.
type Editor struct {
	// Suspend and Resume release and restore the terminal around the editor (TUI-26). May be nil.
	Suspend func() error
	Resume  func() error
}

// Edit writes content to an owner-only temp file in the OS temp dir, runs the
// editor command on it, returns the saved text, and always deletes the file.
func (e Editor) Edit(ctx context.Context, command, content string) (string, error) {
	argv := strings.Fields(command)
	if len(argv) == 0 {
		return "", errors.New("no editor command")
	}
	f, err := os.CreateTemp("", "agent-factory-proposal-*.txt") // mode 0600
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	path := f.Name()
	defer os.Remove(path)
	if err := os.Chmod(path, 0o600); err != nil {
		f.Close()
		return "", fmt.Errorf("setting permissions on temp file: %w", err)
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return "", fmt.Errorf("writing temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("closing temp file: %w", err)
	}

	if e.Suspend != nil {
		if err := e.Suspend(); err != nil {
			return "", fmt.Errorf("suspending the screen: %w", err)
		}
	}
	cmd := exec.CommandContext(ctx, argv[0], append(argv[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	runErr := cmd.Run()
	if e.Resume != nil {
		if err := e.Resume(); err != nil && runErr == nil {
			runErr = fmt.Errorf("restoring the screen: %w", err)
		}
	}
	if runErr != nil {
		return "", fmt.Errorf("editor %q failed: %w", argv[0], runErr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading the edited file: %w", err)
	}
	return string(data), nil
}
