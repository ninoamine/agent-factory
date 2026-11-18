package configuration

import (
	"fmt"
	"sort"
)

// ModelsFile is the typed content of models.json.
type ModelsFile struct {
	Providers map[string]ProviderEntry `json:"providers"`
	Models    []ModelEntry             `json:"models"`
}

// ProviderEntry holds provider settings. APIKey must never be printed (SEC-1).
type ProviderEntry struct {
	Type           string `json:"type"`                     // PROV-1
	BaseURL        string `json:"baseUrl,omitempty"`        // PROV-2, PROV-3
	APIKey         string `json:"apiKey"`                   // PROV-4
	TimeoutSeconds *int   `json:"timeoutSeconds,omitempty"` // PROV-7
}

// ModelEntry is one user-declared model.
type ModelEntry struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	// Model is the exact name sent to the provider API; the id when absent (PROV-8).
	Model *string `json:"model,omitempty"`
	// MaxOutputTokens limits the answer length when set (PROV-9).
	MaxOutputTokens *int          `json:"maxOutputTokens,omitempty"`
	Metadata        *UserMetadata `json:"metadata,omitempty"`
}

// APIName returns the model name sent to the provider API (PROV-8).
func (m ModelEntry) APIName() string {
	if m.Model != nil {
		return *m.Model
	}
	return m.ID
}

// UserMetadata is optional; every field may be missing (CFG-9).
type UserMetadata struct {
	Capabilities []string `json:"capabilities,omitempty"`
	Speed        string   `json:"speed,omitempty"`
	Pricing      *Pricing `json:"pricing,omitempty"`
}

// Pricing is USD per one million tokens. A nil value is unknown; 0 is free.
type Pricing struct {
	InputPer1M  *float64 `json:"inputPer1M,omitempty"`
	OutputPer1M *float64 `json:"outputPer1M,omitempty"`
}

// Known reports whether at least one price is set.
func (p *Pricing) Known() bool { return p != nil && (p.InputPer1M != nil || p.OutputPer1M != nil) }

// Validate rejects negative prices.
func (p *Pricing) Validate() string {
	if p == nil {
		return ""
	}
	if p.InputPer1M != nil && *p.InputPer1M < 0 {
		return "inputPer1M must not be negative"
	}
	if p.OutputPer1M != nil && *p.OutputPer1M < 0 {
		return "outputPer1M must not be negative"
	}
	return ""
}

// Validate checks the content and returns every problem found as "path: reason".
func (m *ModelsFile) Validate() []FieldError {
	var errs []FieldError
	names := make([]string, 0, len(m.Providers))
	for name := range m.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "" {
			errs = append(errs, FieldError{"providers", "provider name must not be empty"})
		}
		errs = append(errs, m.Providers[name].validate("providers."+name)...)
	}
	seen := map[string]bool{}
	for i, md := range m.Models {
		p := fmt.Sprintf("models[%d]", i)
		if md.ID == "" {
			errs = append(errs, FieldError{p + ".id", "must not be empty"})
		} else if seen[md.ID] {
			errs = append(errs, FieldError{p + ".id", fmt.Sprintf("duplicate model id %q", md.ID)})
		}
		seen[md.ID] = true
		if md.Model != nil && *md.Model == "" {
			errs = append(errs, FieldError{p + ".model", "must not be empty when set"})
		}
		if md.MaxOutputTokens != nil && *md.MaxOutputTokens <= 0 {
			errs = append(errs, FieldError{p + ".maxOutputTokens", "must be a positive integer"})
		}
		if md.Metadata != nil {
			if r := md.Metadata.Pricing.Validate(); r != "" {
				errs = append(errs, FieldError{p + ".metadata.pricing", r})
			}
		}
		if md.Provider == "" {
			errs = append(errs, FieldError{p + ".provider", "must not be empty"})
		} else if _, ok := m.Providers[md.Provider]; !ok {
			errs = append(errs, FieldError{p + ".provider", fmt.Sprintf("provider %q is not declared in providers", md.Provider)})
		}
	}
	return errs
}
