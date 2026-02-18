package requests

import (
	"github.com/PeladoCollado/imager/types"
	"os"
	"path/filepath"
	"testing"
)

func TestStreamReaderLoopsAtEOF(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "requests.json")
	content := `{"method":"GET","path":"/first"}
{"method":"POST","path":"/second","body":"payload"}`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("unable to write temp request file: %v", err)
	}

	source, err := NewFileReader(file)
	if err != nil {
		t.Fatalf("unable to create stream reader: %v", err)
	}

	first, err := source.Next()
	if err != nil {
		t.Fatalf("unexpected error reading first request: %v", err)
	}
	second, err := source.Next()
	if err != nil {
		t.Fatalf("unexpected error reading second request: %v", err)
	}
	looped, err := source.Next()
	if err != nil {
		t.Fatalf("unexpected error reading looped request: %v", err)
	}

	assertRequest(t, first, "GET", "/first")
	assertRequest(t, second, "POST", "/second")
	assertRequest(t, looped, "GET", "/first")
}

func assertRequest(t *testing.T, req types.RequestSpec, method string, path string) {
	t.Helper()
	if req.Method != method {
		t.Fatalf("expected method %s, got %s", method, req.Method)
	}
	if req.Path != path {
		t.Fatalf("expected path %s, got %s", path, req.Path)
	}
}
