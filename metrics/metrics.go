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
	RecordJobPickedUp(requestCount int)
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
		jobsPickedUp: prometheus.NewCounter(prometheus.CounterOpts{Name: "executor_jobs_picked_up_total",
			Namespace: "imager",
			Help:      "Number of jobs picked up by this executor"}),
		jobRequestCount: prometheus.NewCounter(prometheus.CounterOpts{Name: "executor_job_requests_total",
			Namespace: "imager",
			Help:      "Number of requests specified in jobs picked up by this executor"}),
	}
	r.MustRegister(
		c.duration,
		c.successDuration,
		c.responseSize,
		c.firstByteDuration,
		c.successCounter,
		c.failedCounter,
		c.jobsPickedUp,
		c.jobRequestCount,
	)
	return c
}

func timeBuckets() []float64 {
	buckets := make([]float64, 0, 128)
	for bucket := float64(10); ; {
		buckets = append(buckets, bucket)
		if bucket < 100 {
			bucket += 5
		} else if bucket < 1000 {
			bucket += 25
		} else if bucket < 10000 {
			bucket += 100
		} else if bucket < 60000 {
			bucket += 1000
		} else {
			break
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
	jobsPickedUp      prometheus.Counter
	jobRequestCount   prometheus.Counter
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

func (b *PrometheusMetricsCollector) RecordJobPickedUp(requestCount int) {
	b.jobsPickedUp.Inc()
	b.jobRequestCount.Add(float64(requestCount))
}

type OrchestratorMetrics struct {
	jobsDispatched      prometheus.Counter
	jobRequestCount     prometheus.Counter
	registeredExecutors prometheus.Gauge
	targetPodCPU        *prometheus.GaugeVec
	targetPodMemory     *prometheus.GaugeVec
}

func NewOrchestratorMetrics(r prometheus.Registerer) *OrchestratorMetrics {
	metrics := &OrchestratorMetrics{
		jobsDispatched: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "imager",
			Name:      "orchestrator_jobs_dispatched_total",
			Help:      "Total number of jobs dispatched by the orchestrator.",
		}),
		jobRequestCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "imager",
			Name:      "orchestrator_job_requests_total",
			Help:      "Total number of requests specified across dispatched jobs.",
		}),
		registeredExecutors: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "imager",
			Name:      "orchestrator_registered_executors",
			Help:      "Number of executors currently registered with the orchestrator.",
		}),
		targetPodCPU: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "imager",
			Name:      "orchestrator_target_pod_cpu_millicores",
			Help:      "CPU usage per monitored target pod in millicores.",
		}, []string{"namespace", "pod"}),
		targetPodMemory: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "imager",
			Name:      "orchestrator_target_pod_memory_bytes",
			Help:      "Memory usage per monitored target pod in bytes.",
		}, []string{"namespace", "pod"}),
	}
	r.MustRegister(
		metrics.jobsDispatched,
		metrics.jobRequestCount,
		metrics.registeredExecutors,
		metrics.targetPodCPU,
		metrics.targetPodMemory,
	)
	return metrics
}

func (o *OrchestratorMetrics) SetRegisteredExecutors(count int) {
	o.registeredExecutors.Set(float64(count))
}

func (o *OrchestratorMetrics) RecordJobDispatched(requestCount int) {
	o.jobsDispatched.Inc()
	o.jobRequestCount.Add(float64(requestCount))
}

func (o *OrchestratorMetrics) SetTargetPodUsage(namespace string, podName string, cpuMillicores int64, memoryBytes int64) {
	o.targetPodCPU.WithLabelValues(namespace, podName).Set(float64(cpuMillicores))
	o.targetPodMemory.WithLabelValues(namespace, podName).Set(float64(memoryBytes))
}

func (o *OrchestratorMetrics) ResetTargetPodUsage() {
	o.targetPodCPU.Reset()
	o.targetPodMemory.Reset()
}
