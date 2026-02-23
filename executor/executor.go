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
	var orchestratorPort int
	var workers int
	var metricsPort int
	flag.StringVar(&orchestratorHost, "host", "imgr-orchestrator",
		"The hostname of the orchestrator process")
	flag.IntVar(&orchestratorPort, "port", 8099, "The port of the orchestrator process")
	flag.IntVar(&workers, "workers", 1, "The number of worker threads to start")
	flag.IntVar(&metricsPort, "metrics-port", 9100, "The port to expose executor metrics on")
	flag.Parse()

	workerUuid, err := uuid.NewRandom()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to generate executor id: %v", err)
		os.Exit(1)
	}
	workerId = types.WorkerId{Id: workerUuid.String(), Workers: workers}

	collector := metrics.NewPrometheusMetricsCollector(prometheus.DefaultRegisterer)
	go serveMetrics(metricsPort)

	hostString := fmt.Sprintf("%s:%d", orchestratorHost, orchestratorPort)
	connectURL := fmt.Sprintf("http://%s/connect", hostString)
	heartbeatURL := fmt.Sprintf("http://%s/heartbeat", hostString)
	nextURL := fmt.Sprintf("http://%s/next", hostString)
	reportURL := fmt.Sprintf("http://%s/report", hostString)

	if err := connect(connectURL, workerId); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelChan := make(chan error, 1)

	go func() {
		<-cancelChan
		cancel()
	}()

	go heartbeat(ctx, cancelChan, heartbeatURL)

	workChan := make(chan types.Job)
	for i := 0; i < workers; i++ {
		go runJob(ctx, workChan, collector, reportURL)
	}

	poll(ctx, nextURL, workChan, cancelChan)
}

func serveMetrics(port int) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Logger.Error("Executor metrics server failed", err)
	}
}

func connect(connectURL string, workerId types.WorkerId) error {
	req, err := newWorkerRequest(http.MethodPost, connectURL, workerId)
	if err != nil {
		return fmt.Errorf("unable to encode worker payload: %w", err)
	}
	resp, err := orchestratorClient.Do(req)
	if err != nil {
		return fmt.Errorf("unable to connect to orchestrator at %s - %w", connectURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errMsg := readBody(resp.Body)
		return fmt.Errorf("unable to connect to orchestrator at %s status code %d - %s",
			connectURL, resp.StatusCode, errMsg)
	}
	return nil
}

func poll(ctx context.Context, nextURL string, work chan types.Job, cancelChan chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		req, err := newWorkerRequest(http.MethodPost, nextURL, workerId)
		if err != nil {
			cancelChan <- fmt.Errorf("unable to create next request payload %w", err)
			return
		}
		req = req.WithContext(ctx)

		resp, err := orchestratorClient.Do(req)
		if err != nil {
			cancelChan <- fmt.Errorf("unable to get job from orchestrator %w", err)
			return
		}
		if resp.StatusCode == http.StatusNoContent {
			_ = resp.Body.Close()
			cancelChan <- errors.New("status complete")
			return
		}
		if resp.StatusCode == http.StatusServiceUnavailable {
			_ = resp.Body.Close()
			cancelChan <- errors.New("orchestrator shutting down")
			return
		}
		if resp.StatusCode != http.StatusOK {
			errorMsg := readBody(resp.Body)
			_ = resp.Body.Close()
			cancelChan <- fmt.Errorf("error response fetching job from orchestrator: %d - %s",
				resp.StatusCode, errorMsg)
			return
		}
		decoder := json.NewDecoder(resp.Body)
		jobs := make([]types.Job, 0, workerId.Workers)
		if err := decoder.Decode(&jobs); err != nil {
			_ = resp.Body.Close()
			cancelChan <- fmt.Errorf("unable to decode jobs response: %w", err)
			return
		}
		_ = resp.Body.Close()
		for _, job := range jobs {
			work <- job
		}
	}
}

func runJob(ctx context.Context,
	work chan types.Job,
	metricsCollector metrics.MetricsCollector,
	reportURL string) {
	for {
		select {
		case job := <-work:
			report := worker.RunJob(ctx, job, metricsCollector)
			report.ExecutorID = workerId.Id
			if err := reportJob(ctx, reportURL, report); err != nil {
				logger.Logger.Warn("Unable to report job execution summary", err, report.JobID)
			}
		case <-ctx.Done():
			return
		}
	}
}

var heartbeatError = errors.New("unable to publish heartbeat")

func heartbeat(ctx context.Context, cancel chan error, heartbeatURL string) {
	ticker := time.NewTicker(manager.HeartbeatFrequencySeconds * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			req, err := newWorkerRequest(http.MethodPost, heartbeatURL, workerId)
			if err != nil {
				cancel <- err
				return
			}
			req = req.WithContext(ctx)
			resp, err := orchestratorClient.Do(req)
			if err != nil || resp.StatusCode != http.StatusOK {
				cancel <- heartbeatError
				if resp != nil {
					_ = resp.Body.Close()
				}
				return
			}
			_ = resp.Body.Close()
		case <-ctx.Done():
			return
		}
	}
}

func newWorkerRequest(method string, endpoint string, worker types.WorkerId) (*http.Request, error) {
	payload, err := json.Marshal(worker)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func reportJob(ctx context.Context, reportURL string, report types.JobReport) error {
	payload, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("unable to encode report payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reportURL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("unable to create report request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := orchestratorClient.Do(req)
	if err != nil {
		return fmt.Errorf("unable to publish report: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected report response %d: %s", resp.StatusCode, readBody(resp.Body))
	}
	return nil
}

func readBody(body io.Reader) string {
	bytesRead, err := io.ReadAll(io.LimitReader(body, 10000))
	if err != nil {
		return fmt.Sprintf("unable to read response body: %v", err)
	}
	return string(bytesRead)
}
