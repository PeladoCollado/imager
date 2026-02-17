package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/PeladoCollado/imager/executor/worker"
	"github.com/PeladoCollado/imager/metrics"
	"github.com/PeladoCollado/imager/orchestrator/logger"
	"github.com/PeladoCollado/imager/orchestrator/manager"
	"github.com/PeladoCollado/imager/types"
	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

var orchestratorClient = newClient()

func newClient() *http.Client {
	client := retryablehttp.NewClient()
	client.RetryMax = 5
	client.RetryWaitMin = 100 * time.Millisecond
	return client.StandardClient()
}

var workerId types.WorkerId

func main() {
	var orchestratorHost string
	var port, workers int
	flag.StringVar(&orchestratorHost, "host", "imgr-orchestrator",
		"The hostname of the orchestrator process")
	flag.IntVar(&port, "port", 8099, "The port of the orchestrator process")
	flag.IntVar(&workers, "workers", 1, "The number of worker threads to start")
	flag.Parse()
	hostString := fmt.Sprintf("%s:%d", orchestratorHost, port)
	connectUrl := fmt.Sprintf("http://%s/connect", hostString)

	workerUuid, err := uuid.NewRandom()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to generate executor id: %v", err)
		os.Exit(1)
	}
	workerId = types.WorkerId{Id: workerUuid.String(), Workers: workers}
	buffer := bytes.NewBuffer(make([]byte, 0, 1000))
	jsonEncoder := json.NewEncoder(buffer)
	err = jsonEncoder.Encode(workerId)
	if err != nil {
		logger.Logger.Error("Unable to encode worker as json", err)
		os.Exit(1)
	}
	resp, err := orchestratorClient.Post(connectUrl, "application/javascript", buffer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to orchestrator at %s:%d - %v\n", orchestratorHost, port, err)
		os.Exit(1)
	}
	if resp.StatusCode != 200 {
		errMsg, err := io.ReadAll(io.LimitReader(resp.Body, 10000))
		if err != nil {
			errMsg = []byte(fmt.Sprintf("Unable to read error response body - %s", err.Error()))
		}
		fmt.Fprintf(os.Stderr, "Unable to connect to orchestrator at %s:%d status code %d- %s\n",
			orchestratorHost, port, resp.StatusCode, string(errMsg))
		os.Exit(1)
	}
	ctx, cancel := context.WithCancel(context.Background())

	cancelChan := make(chan error)

	// listen for cancellations
	go func() {
		<-cancelChan
		cancel()
	}()

	go heartbeat(ctx, cancelChan, &http.Request{Method: "POST", URL: &url.URL{Host: hostString, Path: "/heartbeat"}})

	collector := metrics.NewPrometheusMetricsCollector(prometheus.DefaultRegisterer)
	http.Handle("/metrics", promhttp.Handler())

	workChan := make(chan types.Job)
	for i := 0; i < workers; i++ {
		go runJob(ctx, workChan, collector)
	}

	poll(ctx, workers, &http.Request{Method: "GET", URL: &url.URL{Host: hostString, Path: "/next"}}, workChan, cancelChan)
}

func poll(ctx context.Context, workers int, req *http.Request, work chan types.Job, cancelChan chan error) {
	request := req.WithContext(ctx)
	buf := bytes.NewBuffer(make([]byte, 0, 1000))
	encoder := json.NewEncoder(buf)
	encoder.Encode(workerId)
	for {
		// reset the body on each request so the buffer starts from 0 each time
		req.Body = io.NopCloser(bytes.NewBuffer(buf.Bytes()))
		resp, err := orchestratorClient.Do(request)
		if err != nil {
			cancelChan <- fmt.Errorf("unable to get job from orchestrator %w", err)
			return
		}
		// orchestrator is telling us the job is done
		if resp.StatusCode == 204 {
			cancelChan <- errors.New("status complete")
			return
		}
		if resp.StatusCode != 200 {
			errorMsg, err := io.ReadAll(io.LimitReader(resp.Body, 10000))
			if err != nil {
				errorMsg = []byte(fmt.Sprintf("Unable to read error message from orchestrator: %v", err))
			}
			cancelChan <- fmt.Errorf("error response fetching job from orchestrator: %d- %s",
				resp.StatusCode,
				string(errorMsg))
			return
		}
		decoder := json.NewDecoder(resp.Body)
		jobs := make([]types.Job, workers)
		decoder.Decode(&jobs)
		for j := range jobs {
			work <- jobs[j]
		}
	}
}

func runJob(ctx context.Context, work chan types.Job, metricsCollector metrics.MetricsCollector) {
	for {
		select {
		case job := <-work:
			worker.RunJob(ctx, job, metricsCollector)
		case <-ctx.Done():
			return
		}
	}
}

var heartbeatError = errors.New("unable to publish heartbeat")

func heartbeat(ctx context.Context, cancel chan error, req *http.Request) {
	ticker := time.NewTicker(manager.HeartbeatFrequencySeconds * time.Second)
	heartbeatFailures := 0
	request := req.WithContext(ctx)
	for {
		select {
		case <-ticker.C:
			resp, err := orchestratorClient.Do(request)
			if err != nil || resp.StatusCode != 200 {
				heartbeatFailures++
			}
			if heartbeatFailures > manager.MaxMissedHeartbeats {
				fmt.Fprintln(os.Stderr, "Unable to publish heartbeat to orchestrator. Canceling executor process")
				cancel <- heartbeatError
			}
		case <-ctx.Done():
			return
		}
	}
}
