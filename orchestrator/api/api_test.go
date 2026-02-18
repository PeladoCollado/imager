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
	t.Cleanup(manager.ResetExecutors)

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
