package manager

import "testing"

func TestStepFunctionLoadCalculator(t *testing.T) {
	calc := NewStepFunctionLoadCalculator(10, 20, 5)

	if got := calc.Next(); got != 10 {
		t.Fatalf("expected first value 10, got %d", got)
	}
	if got := calc.Next(); got != 15 {
		t.Fatalf("expected second value 15, got %d", got)
	}
	if got := calc.Next(); got != 20 {
		t.Fatalf("expected third value 20, got %d", got)
	}
	if got := calc.Next(); got != 20 {
		t.Fatalf("expected capped value 20, got %d", got)
	}
}

func TestExponentialLoadCalculator(t *testing.T) {
	calc := NewExponentialLoadCalculator(2, 10)

	if got := calc.Next(); got != 2 {
		t.Fatalf("expected first value 2, got %d", got)
	}
	if got := calc.Next(); got != 4 {
		t.Fatalf("expected second value 4, got %d", got)
	}
	if got := calc.Next(); got != 8 {
		t.Fatalf("expected third value 8, got %d", got)
	}
	if got := calc.Next(); got != 10 {
		t.Fatalf("expected capped value 10, got %d", got)
	}
}

func TestLogarithmicLoadCalculator(t *testing.T) {
	calc := NewLogarithmicLoadCalculator(1, 1000)

	if got := calc.Next(); got != 1 {
		t.Fatalf("expected first value 1, got %d", got)
	}
	if got := calc.Next(); got != 10 {
		t.Fatalf("expected second value 10, got %d", got)
	}
	if got := calc.Next(); got != 100 {
		t.Fatalf("expected third value 100, got %d", got)
	}
}

func TestAdaptiveExponentialCalculatorLatencyThresholdAndBinarySearch(t *testing.T) {
	calc, ok := NewAdaptiveExponentialLoadCalculator(10, 500, 200).(FeedbackLoadCalculator)
	if !ok {
		t.Fatalf("adaptive calculator should implement feedback interface")
	}

	if got := calc.Next(); got != 10 {
		t.Fatalf("expected first rate 10, got %d", got)
	}
	calc.Observe(LoadObservation{TotalRPS: 10, CompletedRequests: 10, SuccessCount: 10, P99LatencyMillis: 100})
	if got := calc.Next(); got != 20 {
		t.Fatalf("expected second rate 20, got %d", got)
	}

	calc.Observe(LoadObservation{TotalRPS: 20, CompletedRequests: 20, SuccessCount: 20, P99LatencyMillis: 120})
	if got := calc.Next(); got != 40 {
		t.Fatalf("expected third rate 40, got %d", got)
	}

	calc.Observe(LoadObservation{TotalRPS: 40, CompletedRequests: 40, FailureCount: 40, P99LatencyMillis: 350})
	if got := calc.Next(); got != 1 {
		t.Fatalf("expected recovery rate 1 after failure, got %d", got)
	}

	calc.Observe(LoadObservation{TotalRPS: 1, CompletedRequests: 1, SuccessCount: 1, P99LatencyMillis: 20})
	if got := calc.Next(); got != 30 {
		t.Fatalf("expected first binary probe at 30, got %d", got)
	}

	calc.Observe(LoadObservation{TotalRPS: 30, CompletedRequests: 30, FailureCount: 30, P99LatencyMillis: 260})
	if got := calc.Next(); got != 1 {
		t.Fatalf("expected recovery round after probe, got %d", got)
	}

	calc.Observe(LoadObservation{TotalRPS: 1, CompletedRequests: 1, SuccessCount: 1, P99LatencyMillis: 20})
	if got := calc.Next(); got != 20 {
		t.Fatalf("expected settled sustainable rate 20, got %d", got)
	}
}

func TestAdaptiveExponentialCalculatorTimeoutMode(t *testing.T) {
	calc, ok := NewAdaptiveExponentialLoadCalculator(10, 500, 0).(FeedbackLoadCalculator)
	if !ok {
		t.Fatalf("adaptive calculator should implement feedback interface")
	}
	if got := calc.Next(); got != 10 {
		t.Fatalf("expected first rate 10, got %d", got)
	}

	calc.Observe(LoadObservation{TotalRPS: 10, CompletedRequests: 10, TimeoutCount: 4})
	if got := calc.Next(); got != 20 {
		t.Fatalf("expected ramp to 20, got %d", got)
	}

	calc.Observe(LoadObservation{TotalRPS: 20, CompletedRequests: 20, TimeoutCount: 10})
	if got := calc.Next(); got != 1 {
		t.Fatalf("expected recovery at 1 due to 50%% timeout threshold, got %d", got)
	}
}
