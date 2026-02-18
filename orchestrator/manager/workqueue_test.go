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
	t.Cleanup(ResetExecutors)

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
