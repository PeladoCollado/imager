package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestExecutorMetricsCollectorPublishesJobAndRequestMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := NewPrometheusMetricsCollector(registry)

	collector.RecordJobPickedUp(5)
	collector.PostSuccess(SuccessEvent{
		Status:        200,
		ResponseSize:  123,
		Duration:      10 * time.Millisecond,
		FirstByteTime: 3 * time.Millisecond,
	})

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("unable to gather metrics: %v", err)
	}
	assertMetricValue(t, families, "imager_executor_jobs_picked_up_total", 1)
	assertMetricValue(t, families, "imager_executor_job_requests_total", 5)
	assertMetricValue(t, families, "imager_success", 1)
}

func TestOrchestratorMetricsPublishesRegistryAndPodUsage(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := NewOrchestratorMetrics(registry)

	collector.SetRegisteredExecutors(3)
	collector.RecordJobDispatched(10)
	collector.SetTargetPodUsage("default", "pod-a", 250, 1024)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("unable to gather metrics: %v", err)
	}
	assertMetricValue(t, families, "imager_orchestrator_registered_executors", 3)
	assertMetricValue(t, families, "imager_orchestrator_jobs_dispatched_total", 1)
	assertMetricValue(t, families, "imager_orchestrator_job_requests_total", 10)
}

func assertMetricValue(t *testing.T, families []*dto.MetricFamily, name string, expected float64) {
	t.Helper()
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		if len(family.Metric) == 0 {
			t.Fatalf("metric family %s has no metric samples", name)
		}
		metric := family.Metric[0]
		var got float64
		if metric.Gauge != nil {
			got = metric.Gauge.GetValue()
		} else if metric.Counter != nil {
			got = metric.Counter.GetValue()
		} else {
			t.Fatalf("metric family %s is neither gauge nor counter", name)
		}
		if got != expected {
			t.Fatalf("metric %s expected %.2f, got %.2f", name, expected, got)
		}
		return
	}
	t.Fatalf("metric %s not found", name)
}
