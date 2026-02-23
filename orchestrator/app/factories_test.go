package app

import (
	"testing"

	"github.com/PeladoCollado/imager/orchestrator/manager"
	"github.com/PeladoCollado/imager/types"
)

func TestBuiltInRequestSourceRejectsUnsupportedType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RequestSourceType = "database"

	if _, err := NewBuiltInRequestSource(cfg); err == nil {
		t.Fatalf("expected unsupported request-source-type error")
	}
}

func TestBuiltInLoadCalculatorRejectsUnsupportedType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LoadCalculator = "random-spike"

	if _, err := NewBuiltInLoadCalculator(cfg); err == nil {
		t.Fatalf("expected unsupported load-calculator error")
	}
}

func TestBuiltInLoadCalculatorSupportsAdaptiveExponential(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LoadCalculator = "adaptive-exponential"
	cfg.MinRPS = 10
	cfg.MaxRPS = 100
	cfg.AdaptiveMaxLatencyMillis = 250

	calc, err := NewBuiltInLoadCalculator(cfg)
	if err != nil {
		t.Fatalf("unexpected adaptive calculator error: %v", err)
	}
	if calc.Next() != 10 {
		t.Fatalf("expected adaptive calculator to start at min rps")
	}
}

func TestCustomRequestSourceFactoryCanHandleUnknownType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RequestSourceType = "database"

	factory := RequestSourceFactoryFunc(func(cfg Config) (types.RequestSource, error) {
		if cfg.RequestSourceType == "database" {
			return &staticRequestSource{}, nil
		}
		return NewBuiltInRequestSource(cfg)
	})

	source, err := factory.NewRequestSource(cfg)
	if err != nil {
		t.Fatalf("unexpected factory error: %v", err)
	}
	if source == nil {
		t.Fatalf("expected source instance from custom factory")
	}
}

func TestCustomLoadCalculatorFactoryCanHandleUnknownType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LoadCalculator = "random-spike"

	factory := LoadCalculatorFactoryFunc(func(cfg Config) (manager.LoadCalculator, error) {
		if cfg.LoadCalculator == "random-spike" {
			return staticLoadCalculator(250), nil
		}
		return NewBuiltInLoadCalculator(cfg)
	})

	calc, err := factory.NewLoadCalculator(cfg)
	if err != nil {
		t.Fatalf("unexpected factory error: %v", err)
	}
	if calc.Next() != 250 {
		t.Fatalf("expected custom calculator value 250")
	}
}

type staticRequestSource struct{}

func (s *staticRequestSource) Next() (types.RequestSpec, error) {
	return types.RequestSpec{Method: "GET", Path: "/custom"}, nil
}

func (s *staticRequestSource) Reset() error {
	return nil
}

type staticLoadCalculator int

func (s staticLoadCalculator) Next() int {
	return int(s)
}
