package worker

import (
	"bytes"
	"context"
	"fmt"
	"github.com/PeladoCollado/imager/metrics"
	"github.com/PeladoCollado/imager/orchestrator/logger"
	"github.com/PeladoCollado/imager/types"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var client http.Client

func RunJob(ctx context.Context, job types.Job, metricsCollector metrics.MetricsCollector) types.JobReport {
	report := types.JobReport{
		JobID:           job.ID,
		RoundID:         job.RoundID,
		PlannedRequests: job.RequestedCount(),
		LatencyMillis:   make([]int64, 0, job.RequestedCount()),
	}
	metricsCollector.RecordJobPickedUp(job.RequestedCount())

	jobDuration := job.Duration()
	if jobDuration <= 0 {
		jobDuration = time.Second
	}
	if len(job.TargetURLs) == 0 {
		logger.Logger.Error("Job has no target URLs", job.ID)
		report.FailureCount = report.PlannedRequests
		report.TimeoutCount = report.PlannedRequests
		report.CompletedRequests = report.PlannedRequests
		return report
	}

	runCtx, cancel := context.WithTimeout(ctx, jobDuration)
	defer cancel()

	for idx, requestSpec := range job.Requests {
		select {
		case <-runCtx.Done():
			return report
		default:
		}

		target := job.TargetURLs[idx%len(job.TargetURLs)]
		result := executeRequest(runCtx, target, requestSpec, metricsCollector)
		if !result.executed {
			continue
		}
		report.CompletedRequests++
		report.LatencyMillis = append(report.LatencyMillis, result.duration.Milliseconds())
		if result.success {
			report.SuccessCount++
		} else {
			report.FailureCount++
		}
		if result.timeout {
			report.TimeoutCount++
		}
	}
	return report
}

type requestResult struct {
	executed bool
	success  bool
	timeout  bool
	duration time.Duration
}

func executeRequest(ctx context.Context,
	target string,
	requestSpec types.RequestSpec,
	metricsCollector metrics.MetricsCollector) requestResult {
	requestURL, err := buildRequestURL(target, requestSpec.Path, requestSpec.QueryString)
	if err != nil {
		metricsCollector.PostFailure(metrics.ErrorEvent{ErrMsg: err.Error()})
		return requestResult{executed: true}
	}

	method := requestSpec.Method
	if method == "" {
		method = http.MethodGet
	}

	var body io.Reader
	if requestSpec.Body != "" {
		body = bytes.NewBufferString(requestSpec.Body)
	}

	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		metricsCollector.PostFailure(metrics.ErrorEvent{ErrMsg: err.Error()})
		return requestResult{executed: true}
	}
	for key, values := range requestSpec.Headers {
		request.Header[key] = append([]string(nil), values...)
	}

	start := time.Now()
	response, err := client.Do(request)
	firstByteDuration := time.Since(start)
	if err != nil {
		metricsCollector.PostFailure(metrics.ErrorEvent{
			Status:   0,
			ErrMsg:   err.Error(),
			Duration: firstByteDuration,
		})
		return requestResult{
			executed: true,
			timeout:  errorQualifiesAsTimeout(err, firstByteDuration),
			duration: firstByteDuration,
		}
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		errMsg := readErrorBody(response.Body)
		metricsCollector.PostFailure(metrics.ErrorEvent{
			Status:   response.StatusCode,
			ErrMsg:   errMsg,
			Duration: firstByteDuration,
		})
		return requestResult{
			executed: true,
			timeout:  statusQualifiesAsTimeout(response.StatusCode),
			duration: firstByteDuration,
		}
	}

	bytesRead, readErr := io.Copy(io.Discard, response.Body)
	if readErr != nil {
		metricsCollector.PostFailure(metrics.ErrorEvent{
			Status:   response.StatusCode,
			ErrMsg:   readErr.Error(),
			Duration: time.Since(start),
		})
		return requestResult{
			executed: true,
			duration: time.Since(start),
		}
	}

	duration := time.Since(start)
	metricsCollector.PostSuccess(metrics.SuccessEvent{
		Status:        response.StatusCode,
		ResponseSize:  bytesRead,
		Duration:      duration,
		FirstByteTime: firstByteDuration,
	})
	return requestResult{
		executed: true,
		success:  true,
		duration: duration,
	}
}

func buildRequestURL(targetBaseURL string, path string, query string) (string, error) {
	baseURL, err := url.Parse(targetBaseURL)
	if err != nil {
		return "", fmt.Errorf("invalid target URL %q: %w", targetBaseURL, err)
	}
	if !baseURL.IsAbs() {
		return "", fmt.Errorf("target URL must be absolute: %s", targetBaseURL)
	}

	relativePath := path
	if relativePath == "" {
		relativePath = "/"
	}
	relativeURL := &url.URL{Path: relativePath, RawQuery: query}
	resolved := baseURL.ResolveReference(relativeURL)

	// Avoid accidental double slashes after host while preserving explicit path intent.
	resolved.Path = strings.ReplaceAll(resolved.Path, "//", "/")
	return resolved.String(), nil
}

func readErrorBody(body io.Reader) string {
	limit := io.LimitReader(body, 3000)
	bytesRead, err := io.ReadAll(limit)
	if err != nil {
		return fmt.Sprintf("Unable to read error message from response %v", err)
	}
	return string(bytesRead)
}

func statusQualifiesAsTimeout(statusCode int) bool {
	return statusCode == http.StatusServiceUnavailable || statusCode == http.StatusGatewayTimeout
}

func errorQualifiesAsTimeout(_ error, duration time.Duration) bool {
	if duration >= time.Minute {
		return true
	}
	return false
}
