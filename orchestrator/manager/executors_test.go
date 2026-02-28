package manager

import (
	"testing"
	"time"
)

func TestAddAndGetExecutor(t *testing.T) {
	ResetExecutors()
	t.Cleanup(ResetExecutors)

	AddExecutor("exec-1", 2)
	executor := GetExecutor("exec-1")
	if executor == nil {
		t.Fatalf("expected executor to be present")
	}
	if executor.Workers != 2 {
		t.Fatalf("expected workers=2, got %d", executor.Workers)
	}
}

func TestEligibleExecutorsRemovesStaleEntries(t *testing.T) {
	ResetExecutors()
	t.Cleanup(ResetExecutors)

	AddExecutor("live", 1)
	AddExecutor("stale", 1)

	stale := GetExecutor("stale")
	stale.HeartbeatTime = time.Now().Add(-heartbeatFailureDuration - time.Second)

	eligible := EligibleExecutors()
	if len(eligible) != 1 {
		t.Fatalf("expected 1 eligible executor, got %d", len(eligible))
	}
	if eligible[0].Id != "live" {
		t.Fatalf("expected live executor, got %s", eligible[0].Id)
	}
	if got := GetExecutor("stale"); got != nil {
		t.Fatalf("expected stale executor to be removed")
	}
}

func TestRecordHeartbeatUpdatesTimestamp(t *testing.T) {
	ResetExecutors()
	t.Cleanup(ResetExecutors)

	AddExecutor("exec-1", 1)
	executor := GetExecutor("exec-1")
	before := executor.HeartbeatTime
	time.Sleep(5 * time.Millisecond)

	RecordHeartbeat("exec-1")
	after := GetExecutor("exec-1").HeartbeatTime
	if !after.After(before) {
		t.Fatalf("expected heartbeat time to increase, before=%v after=%v", before, after)
	}
}
