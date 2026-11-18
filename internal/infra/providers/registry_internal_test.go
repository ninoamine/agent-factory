package providers

import (
	"bytes"
	"net/http"
	"testing"
)

func TestPROV21ProxyFromEnvironment(t *testing.T) {
	t.Run("PROV-21 the shared transport reads the proxy environment", func(t *testing.T) {
		tr, ok := NewRegistry().roundTripper().(*http.Transport)
		if !ok || tr.Proxy == nil {
			t.Fatal("transport has no proxy function")
		}
	})
}

func TestPROV31ErrorBodyLimit(t *testing.T) {
	t.Run("PROV-31 at most 64 KiB of an error body is read", func(t *testing.T) {
		got := readLimited(bytes.NewReader(bytes.Repeat([]byte("x"), 1<<20)))
		if len(got) != 64<<10 {
			t.Fatalf("read %d bytes", len(got))
		}
	})
}
