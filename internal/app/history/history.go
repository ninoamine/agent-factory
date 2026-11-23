// Package history keeps minimal local run metadata (HIST). It never stores
// prompt or response text (HIST-3) and never feeds routing or configuration (HIST-8).
package history

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/ninoamine/agent-factory/internal/app/execution"
)

// ErrNotFound is returned when no entry has the requested id.
var ErrNotFound = errors.New("history entry not found")

// Store is the file access the service needs. Implemented in infra/storage.
type Store interface {
	// Read returns the whole file; the error satisfies errors.Is(err, fs.ErrNotExist) when absent.
	Read(ctx context.Context) ([]byte, error)
	// Append adds data to the end of the file, creating it if needed (HIST-9).
	Append(ctx context.Context, data []byte) error
	// Rewrite replaces the whole file atomically (HIST-7).
	Rewrite(ctx context.Context, data []byte) error
}

// Entry is one line of history.jsonl (HIST-1). Token counts are null when unavailable (HIST-4).
type Entry struct {
	ID           string `json:"id"`
	Timestamp    string `json:"timestamp"`
	Command      string `json:"command"`
	Model        string `json:"model"`
	InputTokens  *int   `json:"inputTokens"`
	OutputTokens *int   `json:"outputTokens"`
	Status       string `json:"status"`
	SDD          string `json:"sdd,omitempty"` // HIST-11: subcommand name only, never text
}

// Validate checks an entry read from disk (CONV-5, HIST-2).
func (e Entry) Validate() error {
	if e.ID == "" {
		return errors.New("missing id")
	}
	switch execution.Status(e.Status) {
	case execution.StatusSuccess, execution.StatusError, execution.StatusInterrupted:
	default:
		return fmt.Errorf("invalid status %q", e.Status)
	}
	return nil
}

// Service records, lists and deletes history entries.
type Service struct {
	Store Store
	NewID func() string // defaults to random hex

	mu sync.Mutex
}

// Record implements execution.Recorder (HIST-1). Only metadata is kept; the prompt is dropped.
func (s *Service) Record(ctx context.Context, r execution.Record) error {
	e := Entry{
		ID:           s.newID(),
		Timestamp:    r.Time.UTC().Format(time.RFC3339),
		Command:      string(r.Command),
		Model:        string(r.Model),
		InputTokens:  r.Usage.Input,
		OutputTokens: r.Usage.Output,
		Status:       string(r.Status),
		SDD:          r.SDD,
	}
	if err := e.Validate(); err != nil {
		return fmt.Errorf("recording history entry: %w", err)
	}
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encoding history entry: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Store.Append(ctx, append(line, '\n')); err != nil {
		return fmt.Errorf("writing history.jsonl: %w", err)
	}
	return nil
}

// List returns valid entries and one warning per skipped corrupted line (HIST-5, HIST-10).
func (s *Service) List(ctx context.Context) ([]Entry, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines, err := s.readLines(ctx)
	if err != nil {
		return nil, nil, err
	}
	var entries []Entry
	var warnings []string
	for i, l := range lines {
		e, err := parse(l)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("history.jsonl line %d skipped: %v", i+1, err))
			continue
		}
		entries = append(entries, e)
	}
	return entries, warnings, nil
}

// Delete removes the entry with the given id (HIST-6, HIST-7). Other lines are kept as they are.
func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines, err := s.readLines(ctx)
	if err != nil {
		return err
	}
	var kept []string
	found := false
	for _, l := range lines {
		if e, err := parse(l); err == nil && e.ID == id {
			found = true
			continue
		}
		kept = append(kept, l)
	}
	if !found {
		return fmt.Errorf("deleting history entry %q: %w", id, ErrNotFound)
	}
	return s.rewrite(ctx, kept)
}

// Clear removes all entries (HIST-6, HIST-7). The CLI asks for confirmation before calling it.
func (s *Service) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rewrite(ctx, nil)
}

func (s *Service) rewrite(ctx context.Context, lines []string) error {
	var buf bytes.Buffer
	for _, l := range lines {
		buf.WriteString(l)
		buf.WriteByte('\n')
	}
	if err := s.Store.Rewrite(ctx, buf.Bytes()); err != nil {
		return fmt.Errorf("rewriting history.jsonl: %w", err)
	}
	return nil
}

// readLines returns non-empty lines; an absent file is empty history (HIST-9).
func (s *Service) readLines(ctx context.Context) ([]string, error) {
	data, err := s.Store.Read(ctx)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading history.jsonl: %w", err)
	}
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

func parse(line string) (Entry, error) {
	var e Entry
	dec := json.NewDecoder(strings.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		return Entry{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if err := e.Validate(); err != nil {
		return Entry{}, err
	}
	return e, nil
}

func (s *Service) newID() string {
	if s.NewID != nil {
		return s.NewID()
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
