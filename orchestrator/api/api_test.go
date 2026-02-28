package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/PeladoCollado/imager/orchestrator/manager"
	"github.com/PeladoCollado/imager/types"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConnectHeartbeatAndNext(t *testing.T) {
	manager.ResetExecutors()
	manager.ResetRoundReports()
	t.Cleanup(manager.ResetExecutors)
	t.Cleanup(manager.ResetRoundReports)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := NewHandler(ctx)
	worker := types.WorkerId{Id: "worker-1", Workers: 2}

	connectReq := httptest.NewRequest(http.MethodPost, "/connect", marshalBody(t, worker))
	connectResp := httptest.NewRecorder()
	handler.ServeHTTP(connectResp, connectReq)
	if connectResp.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, connectResp.Code)
	}

	heartbeatReq := httptest.NewRequest(http.MethodPost, "/heartbeat", marshalBody(t, worker))
	heartbeatResp := httptest.NewRecorder()
	handler.ServeHTTP(heartbeatResp, heartbeatReq)
	if heartbeatResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, heartbeatResp.Code)
	}

	exec := manager.GetExecutor(worker.Id)
	exec.WorkChan = make(chan []types.Job, 1)
	expectedJobs := []types.Job{
		{
			ID:             "job-1",
			Requests:       []types.RequestSpec{{Method: "GET", Path: "/a"}},
			TargetURLs:     []string{"http://example:8080"},
			RatePerSec:     1,
			DurationMillis: 1000,
		},
	}
	exec.WorkChan <- expectedJobs

	nextReq := httptest.NewRequest(http.MethodPost, "/next", marshalBody(t, worker))
	nextResp := httptest.NewRecorder()
	handler.ServeHTTP(nextResp, nextReq)
	if nextResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, nextResp.Code)
	}

	var decoded []types.Job
	if err := json.Unmarshal(nextResp.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("unable to decode response: %v", err)
	}
	if len(decoded) != 1 || decoded[0].ID != "job-1" {
		t.Fatalf("unexpected next jobs payload: %+v", decoded)
	}
}

func TestReportEndpointAcceptsJobReports(t *testing.T) {
	manager.ResetRoundReports()
	t.Cleanup(manager.ResetRoundReports)

	handler := NewHandler(context.Background())
	manager.RegisterRound("round-1", 10, 1, 2)
	report := types.JobReport{
		JobID:             "job-1",
		RoundID:           "round-1",
		PlannedRequests:   2,
		CompletedRequests: 2,
		SuccessCount:      2,
		LatencyMillis:     []int64{10, 20},
	}
	req := httptest.NewRequest(http.MethodPost, "/report", marshalBody(t, report))
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, resp.Code)
	}

	observations := manager.DrainReadyObservations(0)
	if len(observations) != 1 {
		t.Fatalf("expected one observation, got %d", len(observations))
	}
	if observations[0].RoundID != "round-1" {
		t.Fatalf("unexpected round id %q", observations[0].RoundID)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	handler := NewHandler(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/connect", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, resp.Code)
	}
}

func marshalBody(t *testing.T, value any) *bytes.Buffer {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("unable to marshal payload: %v", err)
	}
	return bytes.NewBuffer(payload)
}
