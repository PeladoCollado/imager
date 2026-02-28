package manager

import (
	"context"
	"github.com/PeladoCollado/imager/types"
	"testing"
	"time"
)

type staticCalc struct {
	value int
}

func (s *staticCalc) Next() int {
	return s.value
}

type feedbackCalc struct {
	value        int
	observations []LoadObservation
}

func (f *feedbackCalc) Next() int {
	return f.value
}

func (f *feedbackCalc) Observe(observation LoadObservation) {
	f.observations = append(f.observations, observation)
}

type fakeSource struct {
	next int
}

func (f *fakeSource) Next() (types.RequestSpec, error) {
	f.next++
	return types.RequestSpec{
		Method: "GET",
		Path:   "/resource",
	}, nil
}

func (f *fakeSource) Reset() error {
	f.next = 0
	return nil
}

type fakeResolver struct {
	targets []string
}

func (f *fakeResolver) ResolveTargets(context.Context) ([]string, error) {
	return f.targets, nil
}

type fakeScheduleMetrics struct {
	registered int
	dispatched int
}

func (f *fakeScheduleMetrics) SetRegisteredExecutors(count int) {
	f.registered = count
}

func (f *fakeScheduleMetrics) RecordJobDispatched(requestCount int) {
	f.dispatched += requestCount
}

func TestDispatchTickBuildsJobsAndDistributesRequests(t *testing.T) {
	ResetExecutors()
	ResetRoundReports()
	t.Cleanup(ResetExecutors)
	t.Cleanup(ResetRoundReports)

	AddExecutor("executor-1", 2)
	exec := GetExecutor("executor-1")
	exec.WorkChan = make(chan []types.Job, 1)

	calc := &staticCalc{value: 3}
	source := &fakeSource{}
	resolver := &fakeResolver{targets: []string{"http://10.0.0.1:8080"}}
	metrics := &fakeScheduleMetrics{}

	dispatchTick(
		context.Background(),
		calc,
		source,
		resolver,
		metrics,
		time.Second,
	)

	select {
	case jobs := <-exec.WorkChan:
		if len(jobs) != 2 {
			t.Fatalf("expected 2 jobs, got %d", len(jobs))
		}
		if got := len(jobs[0].Requests); got != 2 {
			t.Fatalf("expected first worker to receive 2 requests, got %d", got)
		}
		if got := len(jobs[1].Requests); got != 1 {
			t.Fatalf("expected second worker to receive 1 request, got %d", got)
		}
		if len(jobs[0].TargetURLs) != 1 || jobs[0].TargetURLs[0] != "http://10.0.0.1:8080" {
			t.Fatalf("unexpected targets on job: %+v", jobs[0].TargetURLs)
		}
	default:
		t.Fatalf("expected queued jobs for executor")
	}

	if metrics.registered != 1 {
		t.Fatalf("expected registered executor metric=1, got %d", metrics.registered)
	}
	if metrics.dispatched != 3 {
		t.Fatalf("expected 3 dispatched requests total, got %d", metrics.dispatched)
	}
}

func TestDispatchTickFeedsRoundObservationsToFeedbackCalculator(t *testing.T) {
	ResetExecutors()
	ResetRoundReports()
	t.Cleanup(ResetExecutors)
	t.Cleanup(ResetRoundReports)

	AddExecutor("executor-1", 1)
	exec := GetExecutor("executor-1")
	exec.WorkChan = make(chan []types.Job, 2)

	calc := &feedbackCalc{value: 10}
	source := &fakeSource{}
	resolver := &fakeResolver{targets: []string{"http://10.0.0.1:8080"}}

	dispatchTick(context.Background(), calc, source, resolver, nil, time.Second)
	jobs := <-exec.WorkChan
	if len(jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs))
	}

	if err := RecordJobReport(types.JobReport{
		JobID:             jobs[0].ID,
		RoundID:           jobs[0].RoundID,
		PlannedRequests:   len(jobs[0].Requests),
		CompletedRequests: len(jobs[0].Requests),
		SuccessCount:      len(jobs[0].Requests),
		LatencyMillis:     []int64{10, 20, 30},
	}); err != nil {
		t.Fatalf("unexpected report error: %v", err)
	}

	dispatchTick(context.Background(), calc, source, resolver, nil, time.Second)

	if len(calc.observations) != 1 {
		t.Fatalf("expected one observation callback, got %d", len(calc.observations))
	}
	if calc.observations[0].RoundID != jobs[0].RoundID {
		t.Fatalf("expected observed round id %s, got %s", jobs[0].RoundID, calc.observations[0].RoundID)
	}
}
