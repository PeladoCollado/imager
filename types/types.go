package types

import (
	"time"
)

type RequestSpec struct {
	Method      string              `json:"method"`
	Path        string              `json:"path"`
	QueryString string              `json:"queryString,omitempty"`
	Headers     map[string][]string `json:"headers,omitempty"`
	Body        string              `json:"body,omitempty"`
}

type Job struct {
	ID             string        `json:"id"`
	RoundID        string        `json:"roundId,omitempty"`
	Requests       []RequestSpec `json:"requests"`
	TargetURLs     []string      `json:"targetUrls"`
	RatePerSec     int           `json:"ratePerSec"`
	DurationMillis int64         `json:"durationMillis"`
}

type RequestSource interface {
	Next() (RequestSpec, error)
	Reset() error
}

func (j Job) Duration() time.Duration {
	return time.Duration(j.DurationMillis) * time.Millisecond
}

func (j Job) RequestedCount() int {
	return len(j.Requests)
}

type WorkerId struct {
	Id      string `json:"id"`
	Workers int    `json:"workers"`
}

type JobReport struct {
	ExecutorID        string  `json:"executorId,omitempty"`
	JobID             string  `json:"jobId"`
	RoundID           string  `json:"roundId"`
	PlannedRequests   int     `json:"plannedRequests"`
	CompletedRequests int     `json:"completedRequests"`
	SuccessCount      int     `json:"successCount"`
	FailureCount      int     `json:"failureCount"`
	TimeoutCount      int     `json:"timeoutCount"`
	LatencyMillis     []int64 `json:"latencyMillis,omitempty"`
}
