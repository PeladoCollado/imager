package manager

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/PeladoCollado/imager/types"
)

type roundAggregate struct {
	RoundID         string
	TotalRPS        int
	PlannedRequests int
	HasRoundPlan    bool
	ExpectedReports int
	ReceivedReports int

	SuccessCount      int
	FailureCount      int
	TimeoutCount      int
	CompletedRequests int
	LatencyMillis     []int64

	ReceivedJobIDs map[string]struct{}
	CreatedAt      time.Time
}

type roundTracker struct {
	lock   sync.Mutex
	rounds map[string]*roundAggregate
	order  []string
}

var reportsTracker = &roundTracker{
	rounds: make(map[string]*roundAggregate),
}

func RegisterRound(roundID string, totalRPS int, expectedReports int, plannedRequests int) {
	if roundID == "" {
		return
	}
	reportsTracker.lock.Lock()
	defer reportsTracker.lock.Unlock()
	aggregate, ok := reportsTracker.rounds[roundID]
	if !ok {
		aggregate = &roundAggregate{
			RoundID:         roundID,
			ReceivedJobIDs:  make(map[string]struct{}),
			CreatedAt:       time.Now(),
			LatencyMillis:   make([]int64, 0),
			ExpectedReports: expectedReports,
		}
		reportsTracker.rounds[roundID] = aggregate
		reportsTracker.order = append(reportsTracker.order, roundID)
	}
	aggregate.TotalRPS = totalRPS
	if expectedReports > aggregate.ExpectedReports {
		aggregate.ExpectedReports = expectedReports
	}
	if plannedRequests >= 0 {
		aggregate.PlannedRequests = plannedRequests
		aggregate.HasRoundPlan = true
	}
}

func RecordJobReport(report types.JobReport) error {
	if report.RoundID == "" {
		return fmt.Errorf("roundId is required")
	}
	if report.JobID == "" {
		return fmt.Errorf("jobId is required")
	}
	reportsTracker.lock.Lock()
	defer reportsTracker.lock.Unlock()

	aggregate, ok := reportsTracker.rounds[report.RoundID]
	if !ok {
		aggregate = &roundAggregate{
			RoundID:        report.RoundID,
			ReceivedJobIDs: make(map[string]struct{}),
			CreatedAt:      time.Now(),
			LatencyMillis:  make([]int64, 0),
		}
		reportsTracker.rounds[report.RoundID] = aggregate
		reportsTracker.order = append(reportsTracker.order, report.RoundID)
	}

	if _, duplicate := aggregate.ReceivedJobIDs[report.JobID]; duplicate {
		return nil
	}
	aggregate.ReceivedJobIDs[report.JobID] = struct{}{}

	aggregate.ReceivedReports++
	aggregate.SuccessCount += report.SuccessCount
	aggregate.FailureCount += report.FailureCount
	aggregate.TimeoutCount += report.TimeoutCount
	aggregate.CompletedRequests += report.CompletedRequests
	if !aggregate.HasRoundPlan {
		aggregate.PlannedRequests += max(report.PlannedRequests, 0)
	}
	aggregate.LatencyMillis = append(aggregate.LatencyMillis, report.LatencyMillis...)
	return nil
}

func DrainReadyObservations(staleAfter time.Duration) []LoadObservation {
	if staleAfter <= 0 {
		staleAfter = 2 * time.Second
	}

	reportsTracker.lock.Lock()
	defer reportsTracker.lock.Unlock()

	now := time.Now()
	observations := make([]LoadObservation, 0, len(reportsTracker.order))
	for len(reportsTracker.order) > 0 {
		roundID := reportsTracker.order[0]
		aggregate, ok := reportsTracker.rounds[roundID]
		if !ok {
			reportsTracker.order = reportsTracker.order[1:]
			continue
		}

		complete := aggregate.ExpectedReports > 0 && aggregate.ReceivedReports >= aggregate.ExpectedReports
		stale := now.Sub(aggregate.CreatedAt) >= staleAfter
		if !complete && !stale {
			break
		}

		observation := loadObservationFromAggregate(aggregate)
		observations = append(observations, observation)

		delete(reportsTracker.rounds, roundID)
		reportsTracker.order = reportsTracker.order[1:]
	}
	return observations
}

func ResetRoundReports() {
	reportsTracker.lock.Lock()
	defer reportsTracker.lock.Unlock()
	reportsTracker.rounds = make(map[string]*roundAggregate)
	reportsTracker.order = make([]string, 0)
}

func loadObservationFromAggregate(aggregate *roundAggregate) LoadObservation {
	latencies := append([]int64(nil), aggregate.LatencyMillis...)
	slices.Sort(latencies)

	completed := aggregate.CompletedRequests
	success := aggregate.SuccessCount
	failures := aggregate.FailureCount
	timeouts := aggregate.TimeoutCount
	if aggregate.ReceivedReports == 0 && aggregate.PlannedRequests > 0 {
		// A full round with zero reports is treated as complete timeout failure.
		completed = aggregate.PlannedRequests
		success = 0
		failures = aggregate.PlannedRequests
		timeouts = aggregate.PlannedRequests
	}

	return LoadObservation{
		RoundID:           aggregate.RoundID,
		TotalRPS:          aggregate.TotalRPS,
		PlannedRequests:   aggregate.PlannedRequests,
		CompletedRequests: completed,
		SuccessCount:      success,
		FailureCount:      failures,
		TimeoutCount:      timeouts,
		P99LatencyMillis:  p99Latency(latencies),
	}
}

func p99Latency(sortedLatencies []int64) int64 {
	if len(sortedLatencies) == 0 {
		return 0
	}
	index := (len(sortedLatencies)*99 + 99) / 100
	if index <= 0 {
		index = 1
	}
	if index > len(sortedLatencies) {
		index = len(sortedLatencies)
	}
	return sortedLatencies[index-1]
}
