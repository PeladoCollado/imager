package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSumHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/sum?a=11&b=31", nil)
	resp := httptest.NewRecorder()

	sumHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.Code)
	}
	var body sumResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unable to decode response: %v", err)
	}
	if body.Sum != 42 {
		t.Fatalf("expected sum=42, got %+v", body)
	}
}

func TestSumHandlerMissingParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/sum?a=11", nil)
	resp := httptest.NewRecorder()

	sumHandler(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, resp.Code)
	}
}

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp := httptest.NewRecorder()

	healthHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.Code)
	}
	if got := resp.Body.String(); got != "ok" {
		t.Fatalf("expected ok body, got %q", got)
	}
}
