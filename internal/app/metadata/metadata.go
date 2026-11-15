// Package metadata builds models-metadata.json from API and user metadata.
package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
)

// Source says where a metadata value came from (META-11).
type Source string

const (
	SourceAPI  Source = "api"
	SourceUser Source = "user"
)

// defaultFetchTimeout bounds one model's metadata fetch so a hung provider cannot block startup.
const defaultFetchTimeout = 30 * time.Second

// ErrUnsupported means the provider adapter cannot fetch metadata (META-2).
var ErrUnsupported = errors.New("provider does not support metadata fetching")

// APIMetadata is what an adapter returns. Every field may be empty (unknown).
type APIMetadata struct {
	Capabilities []string
	Speed        string
	Pricing      *configuration.Pricing
}

// Validate checks untrusted provider data (CONV-5).
func (a APIMetadata) Validate() error {
	if r := a.Pricing.Validate(); r != "" {
		return errors.New(r)
	}
	for i, c := range a.Capabilities {
		if strings.TrimSpace(c) == "" {
			return fmt.Errorf("capabilities[%d] is empty", i)
		}
	}
	if a.Speed != "" && strings.TrimSpace(a.Speed) == "" {
		return errors.New("speed is blank")
	}
	return nil
}

// Fetcher is implemented by infra/providers.
type Fetcher interface {
	FetchMetadata(ctx context.Context, provider, modelID string) (APIMetadata, error)
}

// Writer is implemented by infra/storage (FILE-2).
type Writer interface {
	WriteAtomic(ctx context.Context, path string, data []byte) error
}

// Sources records the origin of each known field.
type Sources struct {
	Capabilities Source `json:"capabilities,omitempty"`
	Speed        Source `json:"speed,omitempty"`
	Pricing      Source `json:"pricing,omitempty"`
}

// Entry is one model in models-metadata.json. Unknown fields are absent (META-4).
type Entry struct {
	ID           string                 `json:"id"`
	Capabilities []string               `json:"capabilities,omitempty"`
	Speed        string                 `json:"speed,omitempty"`
	Pricing      *configuration.Pricing `json:"pricing,omitempty"`
	Sources      *Sources               `json:"sources,omitempty"`
}

// HasMetadata reports whether at least one field is known.
func (e Entry) HasMetadata() bool {
	return len(e.Capabilities) > 0 || e.Speed != "" || e.Pricing.Known()
}

// File is the content of models-metadata.json. It never holds API keys (META-6).
type File struct {
	Models []Entry `json:"models"`
}

// Result is what generation produced.
type Result struct {
	File     File
	Warnings []string
	// NoMetadata lists model ids with no metadata; they MUST NOT be put in
	// any strategy (META-5).
	NoMetadata []string
}

// Service generates models-metadata.json.
type Service struct {
	Reader  configuration.FileReader
	Writer  Writer
	Fetcher Fetcher
	// FetchTimeout bounds each model's fetch; zero means defaultFetchTimeout.
	FetchTimeout time.Duration
}

// GenerateIfAbsent generates the file only when it does not exist (META-9).
// generated is false when the file was already there.
func (s Service) GenerateIfAbsent(ctx context.Context, cfg configuration.Config) (res Result, generated bool, err error) {
	_, rerr := s.Reader.Read(ctx, filepath.Join(cfg.Dir, configuration.MetadataFileName))
	if rerr == nil {
		return Result{}, false, nil
	}
	if !errors.Is(rerr, fs.ErrNotExist) {
		if err := ctx.Err(); err != nil {
			return Result{}, false, err
		}
		// Unreadable but present: do not overwrite at startup; /refresh can.
		return Result{Warnings: []string{fmt.Sprintf(
			"%s: the file exists but cannot be read, so it was not regenerated. Fix it, then run /refresh.",
			redact(rerr.Error(), cfg))}}, false, nil
	}
	res, err = s.Generate(ctx, cfg)
	return res, err == nil && cfg.Models != nil, err
}

// Generate merges API and user metadata and writes the file (META-3, META-7).
// It is used at startup when the file is absent and by /refresh.
func (s Service) Generate(ctx context.Context, cfg configuration.Config) (Result, error) {
	var res Result
	if cfg.Models == nil {
		return res, nil // no usable models.json: nothing to describe
	}
	path := filepath.Join(cfg.Dir, configuration.MetadataFileName)
	last, warn := s.lastKnown(ctx, cfg, path)
	res.Warnings = append(res.Warnings, warn...)

	res.File.Models = []Entry{}
	for _, m := range cfg.Models.Models {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		api, w := s.apiFor(ctx, cfg, m, last[m.ID])
		if err := ctx.Err(); err != nil {
			return Result{}, err // cancelled: stop instead of warning for every model
		}
		res.Warnings = append(res.Warnings, w...)
		e := merge(m, api)
		if !e.HasMetadata() {
			res.NoMetadata = append(res.NoMetadata, m.ID)
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"model %q has no metadata: it will not be added to any strategy. Add a metadata block in models.json.", m.ID))
		}
		res.File.Models = append(res.File.Models, e)
	}

	data, err := json.MarshalIndent(res.File, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("encoding %s: %w", configuration.MetadataFileName, err)
	}
	if err := s.Writer.WriteAtomic(ctx, path, append(data, '\n')); err != nil {
		return Result{}, fmt.Errorf("writing %s: %w", configuration.MetadataFileName, err)
	}
	return res, nil
}

// apiFor fetches API metadata for one model, falling back to the last known
// API metadata on failure (META-8).
func (s Service) apiFor(ctx context.Context, cfg configuration.Config, m configuration.ModelEntry, prev APIMetadata) (APIMetadata, []string) {
	if s.Fetcher == nil {
		return APIMetadata{}, nil
	}
	timeout := s.FetchTimeout
	if timeout <= 0 {
		timeout = defaultFetchTimeout
	}
	fctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	api, err := s.Fetcher.FetchMetadata(fctx, m.Provider, m.ID)
	what := "could not fetch metadata"
	if err == nil {
		if err = api.Validate(); err != nil {
			what = "provider returned invalid metadata"
		}
	}
	switch {
	case err == nil:
		return api, nil
	case errors.Is(err, ErrUnsupported):
		return prev, nil // no new API data; keep the last known (META-8)
	}
	msg := redact(err.Error(), cfg)
	return prev, []string{fmt.Sprintf(
		"%s for model %q (%s): keeping last known API metadata. Check the provider, then run /refresh.", what, m.ID, msg)}
}

// merge keeps the user value whenever one is set (META-3, META-4).
func merge(m configuration.ModelEntry, api APIMetadata) Entry {
	e := Entry{ID: m.ID}
	var src Sources
	if m.Metadata != nil && len(m.Metadata.Capabilities) > 0 {
		e.Capabilities, src.Capabilities = append([]string(nil), m.Metadata.Capabilities...), SourceUser
	} else if len(api.Capabilities) > 0 {
		e.Capabilities, src.Capabilities = append([]string(nil), api.Capabilities...), SourceAPI
	}
	if m.Metadata != nil && m.Metadata.Speed != "" {
		e.Speed, src.Speed = m.Metadata.Speed, SourceUser
	} else if api.Speed != "" {
		e.Speed, src.Speed = api.Speed, SourceAPI
	}
	if m.Metadata != nil && m.Metadata.Pricing.Known() {
		e.Pricing, src.Pricing = copyPricing(m.Metadata.Pricing), SourceUser
	} else if api.Pricing.Known() {
		e.Pricing, src.Pricing = copyPricing(api.Pricing), SourceAPI
	}
	if src != (Sources{}) {
		e.Sources = &src
	}
	return e
}

func copyPricing(p *configuration.Pricing) *configuration.Pricing {
	c := configuration.Pricing{}
	if p.InputPer1M != nil {
		v := *p.InputPer1M
		c.InputPer1M = &v
	}
	if p.OutputPer1M != nil {
		v := *p.OutputPer1M
		c.OutputPer1M = &v
	}
	return &c
}

// lastKnown returns the API-sourced values of the previous file. User values
// are ignored: they always come fresh from models.json (META-10).
func (s Service) lastKnown(ctx context.Context, cfg configuration.Config, path string) (map[string]APIMetadata, []string) {
	out := map[string]APIMetadata{}
	data, err := s.Reader.Read(ctx, path)
	if err != nil {
		return out, nil // absent or unreadable: no last known values
	}
	var f File
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return out, []string{redact(fmt.Sprintf("%s is not valid (%v): ignoring previous API metadata.", path, err), cfg)}
	}
	if _, err := dec.Token(); err != io.EOF {
		return out, []string{redact(fmt.Sprintf("%s has unexpected trailing content: ignoring previous API metadata.", path), cfg)}
	}
	var warns []string
	for _, e := range f.Models {
		if e.Sources == nil {
			continue
		}
		if !validSource(e.Sources.Capabilities) || !validSource(e.Sources.Speed) || !validSource(e.Sources.Pricing) {
			warns = append(warns, redact(fmt.Sprintf(
				"%s: model %q has an unknown source value: ignoring its previous API metadata.", path, e.ID), cfg))
			continue
		}
		var a APIMetadata
		if e.Sources.Capabilities == SourceAPI {
			a.Capabilities = e.Capabilities
		}
		if e.Sources.Speed == SourceAPI {
			a.Speed = e.Speed
		}
		if e.Sources.Pricing == SourceAPI {
			a.Pricing = e.Pricing
		}
		if a.Validate() == nil {
			out[e.ID] = a
		}
	}
	return out, warns
}

func validSource(s Source) bool { return s == "" || s == SourceAPI || s == SourceUser }

// redact removes API keys from text (SEC-1).
func redact(s string, cfg configuration.Config) string {
	if cfg.Models == nil {
		return s
	}
	for _, p := range cfg.Models.Providers {
		if p.APIKey != "" {
			s = strings.ReplaceAll(s, p.APIKey, "[redacted]")
		}
	}
	return s
}
