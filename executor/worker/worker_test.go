package worker

import (
	"context"
	"github.com/PeladoCollado/imager/metrics"
	"github.com/PeladoCollado/imager/types"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type fakeMetrics struct {
	lock sync.Mutex

	successes int
	failures  int

	jobsPicked      int
	requestsPlanned int
}

func (f *fakeMetrics) PostSuccess(event metrics.SuccessEvent) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.successes++
}

func (f *fakeMetrics) PostFailure(event metrics.ErrorEvent) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.failures++
}

func (f *fakeMetrics) RecordJobPickedUp(requestCount int) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.jobsPicked++
	f.requestsPlanned += requestCount
}

func TestRunJobExecutesRequestsAgainstTargets(t *testing.T) {
	var server1Count int
	var server2Count int
	var lock sync.Mutex

	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lock.Lock()
		server1Count++
		lock.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lock.Lock()
		server2Count++
		lock.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	job := types.Job{
		ID: "job-1",
		Requests: []types.RequestSpec{
			{Method: "GET", Path: "/one"},
			{Method: "GET", Path: "/two"},
			{Method: "GET", Path: "/three"},
			{Method: "GET", Path: "/four"},
		},
		TargetURLs:     []string{server1.URL, server2.URL},
		DurationMillis: (2 * time.Second).Milliseconds(),
	}
	collector := &fakeMetrics{}

	report := RunJob(context.Background(), job, collector)

	lock.Lock()
	totalCalls := server1Count + server2Count
	lock.Unlock()
	if totalCalls != 4 {
		t.Fatalf("expected 4 total upstream calls, got %d", totalCalls)
	}

	if collector.successes != 4 {
		t.Fatalf("expected 4 success metrics, got %d", collector.successes)
	}
	if collector.failures != 0 {
		t.Fatalf("expected 0 failure metrics, got %d", collector.failures)
	}
	if collector.jobsPicked != 1 {
		t.Fatalf("expected 1 job-picked metric, got %d", collector.jobsPicked)
	}
	if collector.requestsPlanned != 4 {
		t.Fatalf("expected request plan count=4, got %d", collector.requestsPlanned)
	}
	if report.JobID != "job-1" {
		t.Fatalf("unexpected job report id %q", report.JobID)
	}
	if report.CompletedRequests != 4 {
		t.Fatalf("expected completed requests=4, got %d", report.CompletedRequests)
	}
	if report.SuccessCount != 4 || report.FailureCount != 0 {
		t.Fatalf("unexpected success/failure counts in report: %+v", report)
	}
}

func TestRunJobCounts503And504AsTimeouts(t *testing.T) {
	statuses := []int{http.StatusServiceUnavailable, http.StatusGatewayTimeout}
	index := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statuses[index%len(statuses)])
		index++
	}))
	defer server.Close()

	job := types.Job{
		ID: "job-timeout",
		Requests: []types.RequestSpec{
			{Method: "GET", Path: "/one"},
			{Method: "GET", Path: "/two"},
		},
		TargetURLs:     []string{server.URL},
		DurationMillis: time.Second.Milliseconds(),
	}

	report := RunJob(context.Background(), job, &fakeMetrics{})
	if report.TimeoutCount != 2 {
		t.Fatalf("expected timeout count=2, got %d", report.TimeoutCount)
	}
	if report.FailureCount != 2 {
		t.Fatalf("expected failure count=2, got %d", report.FailureCount)
	}
}

func TestBuildRequestURL(t *testing.T) {
	url, err := buildRequestURL("http://example.local:8080", "/hello", "a=b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "http://example.local:8080/hello?a=b"
	if url != expected {
		t.Fatalf("expected %s, got %s", expected, url)
	}
}
