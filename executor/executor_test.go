package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PeladoCollado/imager/types"
)

func TestReportJobPublishesSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/report" {
			t.Fatalf("expected /report path, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	previousClient := orchestratorClient
	orchestratorClient = server.Client()
	t.Cleanup(func() {
		orchestratorClient = previousClient
	})

	err := reportJob(context.Background(), server.URL+"/report", types.JobReport{
		ExecutorID:        "worker-1",
		JobID:             "job-1",
		RoundID:           "round-1",
		PlannedRequests:   2,
		CompletedRequests: 2,
		SuccessCount:      2,
		LatencyMillis:     []int64{5, 7},
	})
	if err != nil {
		t.Fatalf("unexpected report error: %v", err)
	}
}
