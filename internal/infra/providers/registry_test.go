package providers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
	"github.com/ninoamine/agent-factory/internal/infra/providers"
)

type fake struct{}

func (fake) FetchMetadata(context.Context, string) (metadata.APIMetadata, error) {
	return metadata.APIMetadata{Speed: "fast"}, nil
}

func TestMETA2Registry(t *testing.T) {
	r := providers.NewRegistry()
	r.Register("p", fake{})
	t.Run("META-2 routes to the provider adapter", func(t *testing.T) {
		got, err := r.FetchMetadata(context.Background(), "p", "p/x")
		if err != nil || got.Speed != "fast" {
			t.Fatalf("got %+v %v", got, err)
		}
	})
	t.Run("META-2 provider without adapter is unsupported", func(t *testing.T) {
		_, err := r.FetchMetadata(context.Background(), "q", "q/x")
		if !errors.Is(err, metadata.ErrUnsupported) {
			t.Fatalf("err = %v", err)
		}
	})
}

type fakeExec struct{}

func (fakeExec) Execute(context.Context, string, string) (execution.Response, error) {
	return execution.Response{}, nil
}

func TestEXEC1_RegistryResolvesExecutorByProvider(t *testing.T) {
	r := providers.NewRegistry()
	r.RegisterExecutor("p", fakeExec{})
	if _, ok := r.Executor("p"); !ok {
		t.Fatal("registered executor not found")
	}
	if _, ok := r.Executor("other"); ok {
		t.Fatal("unregistered provider must not resolve")
	}
}
