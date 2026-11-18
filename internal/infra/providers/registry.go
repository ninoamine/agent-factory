// Package providers holds provider adapters behind one internal interface.
package providers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/ninoamine/agent-factory/internal/app/configuration"

	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
)

// MetadataAdapter is the optional fetchMetadata part of a provider adapter
// (ARCH-2). SDK types must not leak out of it (ARCH-9).
type MetadataAdapter interface {
	FetchMetadata(ctx context.Context, modelID string) (metadata.APIMetadata, error)
}

// Registry routes metadata fetches and requests to the adapter of each
// provider. Configure rebuilds the adapters from a configuration (PROV-10).
type Registry struct {
	// Version is sent in the User-Agent header (PROV-18).
	Version string

	mu        sync.RWMutex
	adapters  map[string]MetadataAdapter
	executors map[string]execution.Adapter
	transport *http.Transport
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{adapters: map[string]MetadataAdapter{}, executors: map[string]execution.Adapter{}}
}

// Register sets the metadata adapter for a provider name.
func (r *Registry) Register(provider string, a MetadataAdapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[provider] = a
}

// FetchMetadata implements metadata.Fetcher (META-2). A provider without an
// adapter does not support metadata: metadata.ErrUnsupported.
func (r *Registry) FetchMetadata(ctx context.Context, provider, modelID string) (metadata.APIMetadata, error) {
	r.mu.RLock()
	a, ok := r.adapters[provider]
	r.mu.RUnlock()
	if !ok || a == nil {
		return metadata.APIMetadata{}, metadata.ErrUnsupported
	}
	return a.FetchMetadata(ctx, modelID)
}

// RegisterExecutor sets the execution adapter for a provider name (ARCH-2).
func (r *Registry) RegisterExecutor(provider string, a execution.Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[provider] = a
}

// Executor implements execution.Resolver (EXEC-1).
func (r *Registry) Executor(provider string) (execution.Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.executors[provider]
	return a, ok && a != nil
}

// Configure replaces every adapter with new ones built from the providers of
// cfg (PROV-10). Adapters of the previous configuration are not used after
// it returns. A configuration without valid models.json leaves no adapters.
func (r *Registry) Configure(cfg configuration.Config) {
	ex := map[string]execution.Adapter{}
	md := map[string]MetadataAdapter{}
	if cfg.Models != nil {
		for name, p := range cfg.Models.Providers {
			models := map[string]modelInfo{}
			for _, m := range cfg.Models.Models {
				if m.Provider == name {
					models[m.ID] = modelInfo{name: m.APIName(), maxTokens: m.MaxOutputTokens}
				}
			}
			var timeout time.Duration
			if p.TimeoutSeconds != nil {
				timeout = time.Duration(*p.TimeoutSeconds) * time.Second // PROV-7
			}
			b := newBase(name, p.EffectiveBaseURL(), p.APIKey, timeout, r.version(), models, r.roundTripper())
			switch p.Type {
			case configuration.TypeAnthropic:
				a := &anthropicAdapter{base: b}
				ex[name], md[name] = a, a
			case configuration.TypeOpenAI:
				a := &openaiAdapter{base: b}
				ex[name], md[name] = a, a
			case configuration.TypeOllama:
				b.reachHint, b.pullHint = "is Ollama running?", true
				a := &ollamaAdapter{base: b}
				ex[name], md[name] = a, a
			}
		}
	}
	r.mu.Lock()
	r.executors, r.adapters = ex, md
	r.mu.Unlock()
}

func (r *Registry) version() string {
	if r.Version == "" {
		return "dev"
	}
	return r.Version
}

// roundTripper returns one transport shared by all adapters. It honors
// HTTPS_PROXY, HTTP_PROXY and NO_PROXY (PROV-21).
func (r *Registry) roundTripper() http.RoundTripper {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.transport == nil {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.Proxy = http.ProxyFromEnvironment
		r.transport = t
	}
	return r.transport
}
