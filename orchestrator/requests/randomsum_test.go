package requests

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestRandomSumSourceGeneratesBoundedNumbers(t *testing.T) {
	source, err := NewRandomSumSource("/sum", 10, 20)
	if err != nil {
		t.Fatalf("unexpected error constructing source: %v", err)
	}

	for i := 0; i < 100; i++ {
		req, reqErr := source.Next()
		if reqErr != nil {
			t.Fatalf("unexpected source error: %v", reqErr)
		}
		if req.Path != "/sum" {
			t.Fatalf("unexpected request path: %s", req.Path)
		}
		values, parseErr := url.ParseQuery(req.QueryString)
		if parseErr != nil {
			t.Fatalf("unable to parse query string %q: %v", req.QueryString, parseErr)
		}
		assertIntInRange(t, values.Get("a"), 10, 20)
		assertIntInRange(t, values.Get("b"), 10, 20)
	}
}

func TestRandomSumSourceDefaultPath(t *testing.T) {
	source, err := NewRandomSumSource("", 1, 1)
	if err != nil {
		t.Fatalf("unexpected error constructing source: %v", err)
	}
	req, err := source.Next()
	if err != nil {
		t.Fatalf("unexpected source error: %v", err)
	}
	if req.Path != "/sum" {
		t.Fatalf("expected default /sum path, got %s", req.Path)
	}
	if !strings.Contains(req.QueryString, "a=1") || !strings.Contains(req.QueryString, "b=1") {
		t.Fatalf("expected deterministic query values, got %s", req.QueryString)
	}
}

func TestRandomSumSourceRejectsInvalidRange(t *testing.T) {
	if _, err := NewRandomSumSource("/sum", 2, 1); err == nil {
		t.Fatalf("expected invalid range error")
	}
}

func assertIntInRange(t *testing.T, input string, min int, max int) {
	t.Helper()
	value, err := strconv.Atoi(input)
	if err != nil {
		t.Fatalf("unable to parse int value %q: %v", input, err)
	}
	if value < min || value > max {
		t.Fatalf("value out of bounds: %d not in [%d, %d]", value, min, max)
	}
}
