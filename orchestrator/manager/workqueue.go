package manager

import (
	"context"
	"github.com/PeladoCollado/imager/orchestrator/logger"
	"github.com/PeladoCollado/imager/types"
	"time"
)

var workTaskDuration = 30 * time.Second

func Schedule(ctx context.Context,
	calc LoadCalculator,
	source types.RequestSource) {
	go runSchedule(ctx, calc, source)
}

func runSchedule(ctx context.Context,
	calc LoadCalculator,
	source types.RequestSource) {
	ticker := time.NewTicker(workTaskDuration)
	select {
	case <-ctx.Done():
		logger.Logger.Error("Context canceled- canceling all future work", ctx.Err())
		return
	case <-ticker.C:
		totalRps := calc.Next()
		executors := EligibleExecutors()
		workerCount := 0
		for e := range executors {
			workerCount += executors[e].Workers
		}
		avgRps := totalRps / workerCount
		for e := range executors {
			executor := executors[e]
			go func() {
				jobs := make([]types.Job, 0, executor.Workers)

				for i := 0; i < executor.Workers; i++ {
					jobs = append(jobs, types.Job{Duration: workTaskDuration, RatePerSec: avgRps, Source: source})
				}
				executor.WorkChan <- jobs
			}()
		}
	}
}
