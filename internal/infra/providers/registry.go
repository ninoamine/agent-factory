// Package providers holds provider adapters behind one internal interface.
package providers

import (
	"context"

	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
)

// MetadataAdapter is the optional fetchMetadata part of a provider adapter
// (ARCH-2). SDK types must not leak out of it (ARCH-9).
type MetadataAdapter interface {
	FetchMetadata(ctx context.Context, modelID string) (metadata.APIMetadata, error)
}

// Registry routes metadata fetches to the adapter of each provider.
type Registry struct {
	adapters  map[string]MetadataAdapter
	executors map[string]execution.Adapter
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{adapters: map[string]MetadataAdapter{}, executors: map[string]execution.Adapter{}}
}

// Register sets the metadata adapter for a provider name.
func (r *Registry) Register(provider string, a MetadataAdapter) { r.adapters[provider] = a }

// FetchMetadata implements metadata.Fetcher (META-2). A provider without an
// adapter does not support metadata: metadata.ErrUnsupported.
func (r *Registry) FetchMetadata(ctx context.Context, provider, modelID string) (metadata.APIMetadata, error) {
	a, ok := r.adapters[provider]
	if !ok || a == nil {
		return metadata.APIMetadata{}, metadata.ErrUnsupported
	}
	return a.FetchMetadata(ctx, modelID)
}

// RegisterExecutor sets the execution adapter for a provider name (ARCH-2).
func (r *Registry) RegisterExecutor(provider string, a execution.Adapter) { r.executors[provider] = a }

// Executor implements execution.Resolver (EXEC-1).
func (r *Registry) Executor(provider string) (execution.Adapter, bool) {
	a, ok := r.executors[provider]
	return a, ok && a != nil
}
