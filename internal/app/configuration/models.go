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
	APIKey string `json:"apiKey"`
}

// ModelEntry is one user-declared model.
type ModelEntry struct {
	ID       string        `json:"id"`
	Provider string        `json:"provider"`
	Metadata *UserMetadata `json:"metadata,omitempty"`
}

// UserMetadata is optional; every field may be missing (CFG-9).
type UserMetadata struct {
	Capabilities []string `json:"capabilities,omitempty"`
	Speed        string   `json:"speed,omitempty"`
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
		if md.Provider == "" {
			errs = append(errs, FieldError{p + ".provider", "must not be empty"})
		} else if _, ok := m.Providers[md.Provider]; !ok {
			errs = append(errs, FieldError{p + ".provider", fmt.Sprintf("provider %q is not declared in providers", md.Provider)})
		}
	}
	return errs
}
