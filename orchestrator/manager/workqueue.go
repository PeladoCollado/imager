package manager

import (
	"context"
	"fmt"
	"github.com/PeladoCollado/imager/orchestrator/logger"
	"github.com/PeladoCollado/imager/types"
	"time"
)

const (
	DefaultScheduleInterval = time.Second
	DefaultJobDuration      = time.Second
)

type TargetResolver interface {
	ResolveTargets(ctx context.Context) ([]string, error)
}

type ScheduleMetrics interface {
	SetRegisteredExecutors(count int)
	RecordJobDispatched(requestCount int)
}

type ScheduleOptions struct {
	Interval    time.Duration
	JobDuration time.Duration
}

func Schedule(ctx context.Context,
	calc LoadCalculator,
	source types.RequestSource,
	resolver TargetResolver,
	metrics ScheduleMetrics,
	opts ScheduleOptions) {
	go runSchedule(ctx, calc, source, resolver, metrics, opts)
}

func runSchedule(ctx context.Context,
	calc LoadCalculator,
	source types.RequestSource,
	resolver TargetResolver,
	metrics ScheduleMetrics,
	opts ScheduleOptions) {
	interval := opts.Interval
	if interval <= 0 {
		interval = DefaultScheduleInterval
	}
	jobDuration := opts.JobDuration
	if jobDuration <= 0 {
		jobDuration = DefaultJobDuration
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Logger.Error("Context canceled- canceling all future work", ctx.Err())
			return
		case <-ticker.C:
			dispatchTick(ctx, calc, source, resolver, metrics, jobDuration)
		}
	}
}

func dispatchTick(ctx context.Context,
	calc LoadCalculator,
	source types.RequestSource,
	resolver TargetResolver,
	metrics ScheduleMetrics,
	jobDuration time.Duration) {
	if feedbackCalculator, ok := calc.(FeedbackLoadCalculator); ok {
		for _, observation := range DrainReadyObservations(2 * jobDuration) {
			feedbackCalculator.Observe(observation)
		}
	}

	executors := EligibleExecutors()
	if metrics != nil {
		metrics.SetRegisteredExecutors(len(executors))
	}
	if len(executors) == 0 {
		return
	}

	totalWorkers := 0
	for _, executor := range executors {
		totalWorkers += executor.Workers
	}
	if totalWorkers <= 0 {
		logger.Logger.Warn("No registered worker threads are available to receive work")
		return
	}

	targetURLs, err := resolver.ResolveTargets(ctx)
	if err != nil {
		logger.Logger.Error("Unable to resolve load test targets", err)
		return
	}
	if len(targetURLs) == 0 {
		logger.Logger.Warn("No target URLs available for scheduling")
		return
	}

	roundID := fmt.Sprintf("round-%d", time.Now().UnixNano())
	totalRps := calc.Next()
	if totalRps < 0 {
		totalRps = 0
	}

	baseRps := totalRps / totalWorkers
	remainder := totalRps % totalWorkers
	var globalWorkerIndex int
	expectedReports := 0
	plannedRequests := 0

	for _, executor := range executors {
		jobs := make([]types.Job, 0, executor.Workers)
		for i := 0; i < executor.Workers; i++ {
			workerRps := baseRps
			if globalWorkerIndex < remainder {
				workerRps++
			}
			globalWorkerIndex++

			requestCount := int(jobDuration.Seconds()) * workerRps
			if requestCount < 0 {
				requestCount = 0
			}

			requests := make([]types.RequestSpec, 0, requestCount)
			for reqIdx := 0; reqIdx < requestCount; reqIdx++ {
				nextRequest, nextErr := source.Next()
				if nextErr != nil {
					logger.Logger.Error("Unable to retrieve request from source", nextErr)
					break
				}
				requests = append(requests, nextRequest)
			}

			job := types.Job{
				ID:             fmt.Sprintf("%s-%d-%d", executor.Id, time.Now().UnixNano(), i),
				RoundID:        roundID,
				Requests:       requests,
				TargetURLs:     targetURLs,
				RatePerSec:     workerRps,
				DurationMillis: jobDuration.Milliseconds(),
			}
			jobs = append(jobs, job)
			expectedReports++
			plannedRequests += len(requests)
			if metrics != nil {
				metrics.RecordJobDispatched(len(requests))
			}
		}

		if len(jobs) > 0 {
			executor.WorkChan <- jobs
		}
	}

	RegisterRound(roundID, totalRps, expectedReports, plannedRequests)
}
