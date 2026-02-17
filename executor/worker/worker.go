package worker

import (
	"context"
	"fmt"
	"github.com/PeladoCollado/imager/metrics"
	"github.com/PeladoCollado/imager/orchestrator/logger"
	"github.com/PeladoCollado/imager/types"
	"io"
	"log"
	"net/http"
	"time"
)

var client http.Client

func RunJob(ctx context.Context, job types.Job, metricsCollector metrics.MetricsCollector) {
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(job.Duration))
	defer cancel()
	reqChan := make(chan *http.Request)

	// big buffered channels so request handlers aren't blocked on metrics publishing
	respChan := make(chan metrics.SuccessEvent, 1000)
	errChan := make(chan metrics.ErrorEvent, 1000)

	// Once per second up to the job duration, execute a batch of requests up to job.RatePerSec
	go func() {
		ticker := time.NewTicker(time.Second)
		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(ctx, time.Second)

				// request handling is single threaded. to increase parallelization, expect num workers to increase
				go executeRequest(reqChan, errChan, respChan, job.Source, ctx)
				go generateRequests(job, cancel, reqChan, ctx)
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case response := <-respChan:
			metricsCollector.PostSuccess(response)
		case err := <-errChan:
			metricsCollector.PostFailure(err)
		case <-ctx.Done():
			return
		}
	}
}

func generateRequests(job types.Job, cancelFn func(), reqChan chan *http.Request, ctx context.Context) {
	defer cancelFn()
	for i := 0; i < job.RatePerSec; i++ {
		req, err := job.Source.Next()
		if err != nil {
			logger.Logger.Error("Unable to generate new requests", err)
			cancelFn()
			return
		}
		select {
		case reqChan <- req:
			// great! keep looping
		case <-ctx.Done():
			return
		}
	}
}

func executeRequest(reqChan chan *http.Request,
	errChan chan metrics.ErrorEvent,
	respChan chan metrics.SuccessEvent,
	source types.RequestSource,
	ctx context.Context) {

	for {
		select {
		case req := <-reqChan:
			start := time.Now()
			response, err := client.Do(req)
			end := time.Now()
			fbDuration := end.Sub(start)
			if err != nil {
				errChan <- metrics.ErrorEvent{0, err.Error(), fbDuration}
			} else if response.StatusCode >= 300 {
				limit := io.LimitReader(response.Body, 3000) // read the error, but limit the size
				errMsg, err := io.ReadAll(limit)
				if err != nil {
					errMsg = []byte(fmt.Sprint("Unable to read error message from response ", err))
				}
				errChan <- metrics.ErrorEvent{Status: response.StatusCode,
					ErrMsg:   string(errMsg),
					Duration: fbDuration}
			} else {
				bytes, err := source.Read(response)
				if err != nil {
					log.Println("Unable to read response body ", err)
					errChan <- metrics.ErrorEvent{ErrMsg: err.Error(), Duration: fbDuration}
					break
				}
				final := time.Now()
				respChan <- metrics.SuccessEvent{Status: response.StatusCode,
					ResponseSize:  bytes,
					Duration:      final.Sub(start),
					FirstByteTime: fbDuration}
			}
		case <-ctx.Done():
			return
		}
	}
}
