package app

import (
	"fmt"

	"github.com/PeladoCollado/imager/orchestrator/manager"
	"github.com/PeladoCollado/imager/orchestrator/requests"
	"github.com/PeladoCollado/imager/types"
)

type RequestSourceFactory interface {
	NewRequestSource(cfg Config) (types.RequestSource, error)
}

type LoadCalculatorFactory interface {
	NewLoadCalculator(cfg Config) (manager.LoadCalculator, error)
}

type RequestSourceFactoryFunc func(cfg Config) (types.RequestSource, error)

func (f RequestSourceFactoryFunc) NewRequestSource(cfg Config) (types.RequestSource, error) {
	return f(cfg)
}

type LoadCalculatorFactoryFunc func(cfg Config) (manager.LoadCalculator, error)

func (f LoadCalculatorFactoryFunc) NewLoadCalculator(cfg Config) (manager.LoadCalculator, error) {
	return f(cfg)
}

func NewBuiltInRequestSource(cfg Config) (types.RequestSource, error) {
	switch cfg.RequestSourceType {
	case "file":
		if cfg.RequestSourceFile == "" {
			return nil, fmt.Errorf("request-source-file is required when request-source-type=file")
		}
		return requests.NewFileReader(cfg.RequestSourceFile)
	case "random-sum":
		if cfg.RandomSumMax < cfg.RandomSumMin {
			return nil, fmt.Errorf("random-sum-max must be >= random-sum-min")
		}
		return requests.NewRandomSumSource(cfg.RandomSumPath, cfg.RandomSumMin, cfg.RandomSumMax)
	default:
		return nil, fmt.Errorf("unsupported request-source-type %q", cfg.RequestSourceType)
	}
}

func NewBuiltInLoadCalculator(cfg Config) (manager.LoadCalculator, error) {
	switch cfg.LoadCalculator {
	case "step":
		if cfg.StepRPS <= 0 {
			return nil, fmt.Errorf("step-rps must be > 0 for step calculator")
		}
		return manager.NewStepFunctionLoadCalculator(cfg.MinRPS, cfg.MaxRPS, cfg.StepRPS), nil
	case "exponential":
		return manager.NewExponentialLoadCalculator(cfg.MinRPS, cfg.MaxRPS), nil
	case "logarithmic":
		return manager.NewLogarithmicLoadCalculator(cfg.MinRPS, cfg.MaxRPS), nil
	default:
		return nil, fmt.Errorf("unsupported load-calculator %q", cfg.LoadCalculator)
	}
}

func requestSourceFactoryOrDefault(factory RequestSourceFactory) RequestSourceFactory {
	if factory != nil {
		return factory
	}
	return RequestSourceFactoryFunc(NewBuiltInRequestSource)
}

func loadCalculatorFactoryOrDefault(factory LoadCalculatorFactory) LoadCalculatorFactory {
	if factory != nil {
		return factory
	}
	return LoadCalculatorFactoryFunc(NewBuiltInLoadCalculator)
}
