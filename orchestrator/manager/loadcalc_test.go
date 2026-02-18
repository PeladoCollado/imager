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
