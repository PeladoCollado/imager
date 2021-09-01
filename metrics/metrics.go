package metrics

import (
	"time"
)

type SuccessEvent struct {
	Status        int
	ResponseSize  int64
	Duration      time.Duration
	FirstByteTime time.Duration
}

type ErrorEvent struct {
	Status   int
	ErrMsg   string
	Duration time.Duration
}

type MetricsCollector interface {
	PostSuccess(event SuccessEvent)
	PostFailure(event ErrorEvent)
}
