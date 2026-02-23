package app

import (
	"testing"
	"time"
)

func TestValidateConfigAllowsUnknownRequestSourceAndLoadCalculator(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TargetDeployment = "target"
	cfg.RequestSourceType = "database"
	cfg.LoadCalculator = "random-spike"

	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected config to validate for custom factory usage, got: %v", err)
	}
}

func TestValidateConfigRejectsUnsupportedTargetMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TargetMode = "custom"

	if err := ValidateConfig(cfg); err == nil {
		t.Fatalf("expected unsupported target-mode validation error")
	}
}

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig([]string{
		"-listen-port=8111",
		"-target-mode=service",
		"-target-namespace=imager",
		"-target-service=sumservice",
		"-request-source-type=random-sum",
		"-load-calculator=step",
		"-min-rps=10",
		"-max-rps=100",
		"-step-rps=10",
		"-schedule-interval=2s",
		"-job-duration=1s",
		"-metrics-poll-interval=4s",
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if cfg.ListenPort != 8111 {
		t.Fatalf("expected listen port 8111, got %d", cfg.ListenPort)
	}
	if cfg.TargetMode != "service" {
		t.Fatalf("expected service target mode, got %s", cfg.TargetMode)
	}
	if cfg.TargetService != "sumservice" {
		t.Fatalf("expected target service sumservice, got %s", cfg.TargetService)
	}
	if cfg.RequestSourceType != "random-sum" {
		t.Fatalf("expected random-sum request source, got %s", cfg.RequestSourceType)
	}
	if cfg.MinRPS != 10 || cfg.MaxRPS != 100 || cfg.StepRPS != 10 {
		t.Fatalf("unexpected rps values: min=%d max=%d step=%d", cfg.MinRPS, cfg.MaxRPS, cfg.StepRPS)
	}
	if cfg.ScheduleInterval != 2*time.Second {
		t.Fatalf("expected 2s schedule interval, got %s", cfg.ScheduleInterval)
	}
	if cfg.MetricsPollInterval != 4*time.Second {
		t.Fatalf("expected 4s metrics interval, got %s", cfg.MetricsPollInterval)
	}
}
