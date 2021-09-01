package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
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

func NewPrometheusMetricsCollector(r prometheus.Registerer) MetricsCollector {
	c := &PrometheusMetricsCollector{
		duration: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "duration",
			Namespace: "imager",
			Help:      "Request duration",
			Buckets:   timeBuckets()}),
		successDuration: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "successDuration",
			Namespace: "imager",
			Help:      "Successful request duration",
			Buckets:   timeBuckets()}),
		responseSize: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "responseSize",
			Namespace: "imager",
			Help:      "Response size",
			Buckets:   sizeBuckets()}),
		firstByteDuration: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "firstByteDuration",
			Namespace: "imager",
			Help:      "Time to first byte",
			Buckets:   timeBuckets()}),
		successCounter: prometheus.NewCounter(prometheus.CounterOpts{Name: "success",
			Namespace: "imager",
			Help:      "Number of successful requests served"}),
		failedCounter: prometheus.NewCounter(prometheus.CounterOpts{Name: "failed",
			Namespace: "imager",
			Help:      "Number of failed requests"}),
	}
	r.MustRegister(c.duration, c.successDuration, c.responseSize, c.firstByteDuration, c.successCounter, c.failedCounter)
	return c
}

func timeBuckets() []float64 {
	bucket := float64(10)
	buckets := make([]float64, 0, 204)
	for i := 0; i < 204; i++ {
		buckets = append(buckets, bucket)
		if bucket < 100 {
			bucket += 5
		} else if bucket < 1000 {
			bucket += 25
		} else if bucket < 10000 {
			bucket += 100
		} else if bucket < 60000 {
			bucket += 1000
		}
	}
	return buckets
}

const MB = 1 << 20

func sizeBuckets() []float64 {
	bucket := float64(64)
	buckets := make([]float64, 0, 127)
	for bucket < MB {
		buckets = append(buckets, bucket)
		bucket *= 2
	}
	for bucket < 10*MB {
		buckets = append(buckets, bucket)
		bucket += 1024 * 128
	}
	for bucket < 50*MB {
		bucket += MB
	}
	return buckets
}

type PrometheusMetricsCollector struct {
	duration          prometheus.Histogram
	successDuration   prometheus.Histogram
	responseSize      prometheus.Histogram
	firstByteDuration prometheus.Histogram
	successCounter    prometheus.Counter
	failedCounter     prometheus.Counter
}

func (b *PrometheusMetricsCollector) PostSuccess(event SuccessEvent) {
	b.duration.Observe(float64(event.Duration.Milliseconds()))
	b.successDuration.Observe(float64(event.Duration.Milliseconds()))
	b.responseSize.Observe(float64(event.ResponseSize))
	b.firstByteDuration.Observe(float64(event.FirstByteTime.Milliseconds()))
	b.successCounter.Inc()
}

func (b *PrometheusMetricsCollector) PostFailure(event ErrorEvent) {
	b.duration.Observe(float64(event.Duration.Milliseconds()))
	b.failedCounter.Inc()
}
