package app

import (
	"context"
	"testing"
	"time"

	"github.com/PeladoCollado/imager/orchestrator/manager"
	"github.com/PeladoCollado/imager/types"
	"github.com/prometheus/client_golang/prometheus"
)

func TestRunURLModeDoesNotRequireKubernetesConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenPort = 0
	cfg.TargetMode = "url"
	cfg.TargetURL = "https://example.com"
	cfg.TargetNamespace = ""
	cfg.TargetDeployment = ""
	cfg.RequestSourceType = "custom"
	cfg.LoadCalculator = "custom"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, cfg, RunOptions{
			RequestSourceFactory: RequestSourceFactoryFunc(func(cfg Config) (types.RequestSource, error) {
				return &noopSource{}, nil
			}),
			LoadCalculatorFactory: LoadCalculatorFactoryFunc(func(cfg Config) (manager.LoadCalculator, error) {
				return constantLoadCalculator(1), nil
			}),
			Registerer: prometheus.NewRegistry(),
			Gatherer:   prometheus.NewRegistry(),
		})
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected run error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for run to return")
	}
}

type noopSource struct{}

func (n *noopSource) Next() (types.RequestSpec, error) {
	return types.RequestSpec{Method: "GET", Path: "/"}, nil
}

func (n *noopSource) Reset() error {
	return nil
}

type constantLoadCalculator int

func (c constantLoadCalculator) Next() int {
	return int(c)
}
