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

func RunJob(ctx context.Context, job types.Job, metricsCollector metrics.MetricsCollector) {
	metricsCollector.RecordJobPickedUp(job.RequestedCount())

	jobDuration := job.Duration()
	if jobDuration <= 0 {
		jobDuration = time.Second
	}
	if len(job.TargetURLs) == 0 {
		logger.Logger.Error("Job has no target URLs", job.ID)
		return
	}

	runCtx, cancel := context.WithTimeout(ctx, jobDuration)
	defer cancel()

	for idx, requestSpec := range job.Requests {
		select {
		case <-runCtx.Done():
			return
		default:
		}

		target := job.TargetURLs[idx%len(job.TargetURLs)]
		executeRequest(runCtx, target, requestSpec, metricsCollector)
	}
}

func executeRequest(ctx context.Context,
	target string,
	requestSpec types.RequestSpec,
	metricsCollector metrics.MetricsCollector) {
	requestURL, err := buildRequestURL(target, requestSpec.Path, requestSpec.QueryString)
	if err != nil {
		metricsCollector.PostFailure(metrics.ErrorEvent{ErrMsg: err.Error()})
		return
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
		return
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
		return
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		errMsg := readErrorBody(response.Body)
		metricsCollector.PostFailure(metrics.ErrorEvent{
			Status:   response.StatusCode,
			ErrMsg:   errMsg,
			Duration: firstByteDuration,
		})
		return
	}

	bytesRead, readErr := io.Copy(io.Discard, response.Body)
	if readErr != nil {
		metricsCollector.PostFailure(metrics.ErrorEvent{
			Status:   response.StatusCode,
			ErrMsg:   readErr.Error(),
			Duration: time.Since(start),
		})
		return
	}

	metricsCollector.PostSuccess(metrics.SuccessEvent{
		Status:        response.StatusCode,
		ResponseSize:  bytesRead,
		Duration:      time.Since(start),
		FirstByteTime: firstByteDuration,
	})
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
