package manager

import (
	"testing"
	"time"

	"github.com/PeladoCollado/imager/types"
)

func TestRoundReportsAggregateAndDrain(t *testing.T) {
	ResetRoundReports()
	t.Cleanup(ResetRoundReports)

	RegisterRound("round-1", 100, 2, 20)

	if err := RecordJobReport(types.JobReport{
		JobID:             "job-1",
		RoundID:           "round-1",
		PlannedRequests:   10,
		CompletedRequests: 10,
		SuccessCount:      10,
		LatencyMillis:     []int64{10, 20, 30, 40, 50},
	}); err != nil {
		t.Fatalf("unexpected report error: %v", err)
	}
	if err := RecordJobReport(types.JobReport{
		JobID:             "job-2",
		RoundID:           "round-1",
		PlannedRequests:   10,
		CompletedRequests: 10,
		SuccessCount:      9,
		FailureCount:      1,
		TimeoutCount:      1,
		LatencyMillis:     []int64{60, 70, 80, 90, 100},
	}); err != nil {
		t.Fatalf("unexpected report error: %v", err)
	}

	observations := DrainReadyObservations(time.Millisecond)
	if len(observations) != 1 {
		t.Fatalf("expected one observation, got %d", len(observations))
	}
	observation := observations[0]
	if observation.RoundID != "round-1" {
		t.Fatalf("unexpected round id %q", observation.RoundID)
	}
	if observation.CompletedRequests != 20 {
		t.Fatalf("expected completed requests 20, got %d", observation.CompletedRequests)
	}
	if observation.TimeoutCount != 1 {
		t.Fatalf("expected timeout count 1, got %d", observation.TimeoutCount)
	}
	if observation.P99LatencyMillis != 100 {
		t.Fatalf("expected p99 latency 100ms, got %d", observation.P99LatencyMillis)
	}
}

func TestRoundReportsTreatStaleNoReportRoundAsTimeouts(t *testing.T) {
	ResetRoundReports()
	t.Cleanup(ResetRoundReports)

	RegisterRound("round-timeout", 120, 1, 25)
	time.Sleep(2 * time.Millisecond)

	observations := DrainReadyObservations(time.Millisecond)
	if len(observations) != 1 {
		t.Fatalf("expected one observation, got %d", len(observations))
	}
	observation := observations[0]
	if observation.TimeoutCount != 25 {
		t.Fatalf("expected timeout count 25, got %d", observation.TimeoutCount)
	}
	if observation.CompletedRequests != 25 {
		t.Fatalf("expected completed requests 25, got %d", observation.CompletedRequests)
	}
}

func TestRoundReportsIgnoreDuplicateJobReports(t *testing.T) {
	ResetRoundReports()
	t.Cleanup(ResetRoundReports)

	RegisterRound("round-dup", 80, 1, 10)
	report := types.JobReport{
		JobID:             "job-dup",
		RoundID:           "round-dup",
		PlannedRequests:   10,
		CompletedRequests: 10,
		SuccessCount:      10,
		LatencyMillis:     []int64{15, 20},
	}
	if err := RecordJobReport(report); err != nil {
		t.Fatalf("unexpected report error: %v", err)
	}
	if err := RecordJobReport(report); err != nil {
		t.Fatalf("unexpected duplicate report error: %v", err)
	}

	observations := DrainReadyObservations(time.Millisecond)
	if len(observations) != 1 {
		t.Fatalf("expected one observation, got %d", len(observations))
	}
	if observations[0].CompletedRequests != 10 {
		t.Fatalf("expected completed requests 10, got %d", observations[0].CompletedRequests)
	}
}
