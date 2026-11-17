package configuration

import (
	"context"
	"errors"
	"strings"
	"sync"
)

// MetadataRegenerator re-fetches API metadata and regenerates
// models-metadata.json for a validated configuration (REFRESH-2). It is a
// function so this package does not import the metadata package. A fetch
// failure for one model is a returned warning, not an error (META-8).
type MetadataRegenerator func(ctx context.Context, cfg Config) (warnings []string, err error)

// ErrRequestRunning is returned when /refresh is refused because a request is
// running (REFRESH-9).
var ErrRequestRunning = errors.New("a request is running: wait for it to finish, then run /refresh again")

// ErrRefreshInProgress is returned when another /refresh is already running.
var ErrRefreshInProgress = errors.New("a refresh is already running")

// RefreshOutcome is what /refresh shows.
type RefreshOutcome struct {
	// Applied is true when the new configuration became active.
	Applied bool
	// Problems are validation errors per file; the previous configuration stays active (REFRESH-4).
	Problems []Problem
	// Warnings include missing-file warnings (CFG-2) and metadata warnings (REFRESH-5).
	Warnings []string
	// Summary is the configuration summary of the active configuration after success (REFRESH-6).
	Summary string
}

// Manager holds the active configuration. Nothing changes it except a
// successful Refresh (REFRESH-3, REFRESH-7).
type Manager struct {
	reader FileReader
	regen  MetadataRegenerator

	mu         sync.Mutex
	active     Config
	running    int
	refreshing bool
}

// NewManager starts with the startup configuration. regen may be nil, in
// which case no metadata is regenerated.
func NewManager(r FileReader, regen MetadataRegenerator, startup Config) *Manager {
	return &Manager{reader: r, regen: regen, active: startup}
}

// Active returns the active configuration.
func (m *Manager) Active() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

// BeginRequest marks a request as running and returns the configuration it
// must use plus a release function to call when it finishes (REFRESH-9).
func (m *Manager) BeginRequest() (cfg Config, release func()) {
	m.mu.Lock()
	m.running++
	cfg = m.active
	m.mu.Unlock()
	var once sync.Once
	return cfg, func() {
		once.Do(func() {
			m.mu.Lock()
			m.running--
			m.mu.Unlock()
		})
	}
}

// Refresh re-reads models.json and strategies.json, validates them, and only
// when everything validates regenerates metadata and swaps the new
// configuration in (REFRESH-1 to REFRESH-6, REFRESH-9). It never writes the
// user's files (REFRESH-8). Only context cancellation or a metadata write
// failure returns an error; the previous configuration then stays active.
func (m *Manager) Refresh(ctx context.Context) (RefreshOutcome, error) {
	m.mu.Lock()
	switch {
	case m.running > 0:
		m.mu.Unlock()
		return RefreshOutcome{}, ErrRequestRunning
	case m.refreshing:
		m.mu.Unlock()
		return RefreshOutcome{}, ErrRefreshInProgress
	}
	m.refreshing = true
	dir := m.active.Dir
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.refreshing = false
		m.mu.Unlock()
	}()

	res, err := Load(ctx, m.reader, dir) // new configuration built completely first (REFRESH-3)
	if err != nil {
		return RefreshOutcome{}, err
	}
	out := RefreshOutcome{Problems: res.Problems, Warnings: res.Warnings}
	if len(res.Problems) > 0 {
		return out, nil // REFRESH-4: previous configuration stays active
	}

	if m.regen != nil {
		warns, err := m.regen(ctx, res.Config)
		if err != nil {
			return RefreshOutcome{}, err
		}
		out.Warnings = append(out.Warnings, warns...) // REFRESH-5: warnings do not fail the refresh
	}

	m.mu.Lock()
	m.active = res.Config
	m.mu.Unlock()
	out.Applied = true
	out.Summary = res.Config.Summary()
	return out, nil
}

// String formats the outcome for display, with errors per file (REFRESH-4).
func (o RefreshOutcome) String() string {
	var b strings.Builder
	if !o.Applied {
		b.WriteString("Refresh failed: the previous configuration stays active.\n")
		for _, p := range o.Problems {
			b.WriteString("  " + p.String() + "\n")
		}
	} else {
		b.WriteString("Refresh done.\n")
	}
	for _, w := range o.Warnings {
		b.WriteString("Warning: " + w + "\n")
	}
	if o.Applied {
		b.WriteString(o.Summary)
	}
	return b.String()
}
