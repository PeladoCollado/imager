package app

import (
	"flag"
	"fmt"
	"time"

	"github.com/PeladoCollado/imager/orchestrator/k8s"
)

type Config struct {
	ListenPort int

	TargetMode       string
	TargetNamespace  string
	TargetDeployment string
	TargetService    string
	TargetPortName   string
	TargetScheme     string

	RequestSourceType string
	RequestSourceFile string
	RandomSumPath     string
	RandomSumMin      int
	RandomSumMax      int

	LoadCalculator string
	MinRPS         int
	MaxRPS         int
	StepRPS        int

	ScheduleInterval    time.Duration
	JobDuration         time.Duration
	MetricsPollInterval time.Duration

	InCluster  bool
	Kubeconfig string
}

func DefaultConfig() Config {
	return Config{
		ListenPort: 8099,

		TargetMode:      string(k8s.TargetModePod),
		TargetNamespace: "default",
		TargetPortName:  "http",
		TargetScheme:    "http",

		RequestSourceType: "file",
		RequestSourceFile: "/config/requests.json",
		RandomSumPath:     "/sum",
		RandomSumMin:      1,
		RandomSumMax:      100,

		LoadCalculator: "step",
		MinRPS:         1,
		MaxRPS:         100,
		StepRPS:        1,

		ScheduleInterval:    time.Second,
		JobDuration:         time.Second,
		MetricsPollInterval: 5 * time.Second,

		InCluster: true,
	}
}

func BindFlags(fs *flag.FlagSet, cfg *Config) {
	fs.IntVar(&cfg.ListenPort, "listen-port", cfg.ListenPort, "Orchestrator API and metrics port")

	fs.StringVar(&cfg.TargetMode, "target-mode", cfg.TargetMode, "Target mode: pod or service")
	fs.StringVar(&cfg.TargetNamespace, "target-namespace", cfg.TargetNamespace, "Kubernetes namespace for the target")
	fs.StringVar(&cfg.TargetDeployment, "target-deployment", cfg.TargetDeployment, "Target deployment name (pod mode)")
	fs.StringVar(&cfg.TargetService, "target-service", cfg.TargetService, "Target service name (service mode)")
	fs.StringVar(&cfg.TargetPortName, "target-port-name", cfg.TargetPortName, "Target container port name")
	fs.StringVar(&cfg.TargetScheme, "target-scheme", cfg.TargetScheme, "Target request URL scheme")

	fs.StringVar(&cfg.RequestSourceType, "request-source-type", cfg.RequestSourceType, "Request source type: file or random-sum")
	fs.StringVar(&cfg.RequestSourceFile, "request-source-file", cfg.RequestSourceFile, "Path to request source JSON file")
	fs.StringVar(&cfg.RandomSumPath, "random-sum-path", cfg.RandomSumPath, "Path to call when using the random-sum request source")
	fs.IntVar(&cfg.RandomSumMin, "random-sum-min", cfg.RandomSumMin, "Minimum random value used by random-sum request source")
	fs.IntVar(&cfg.RandomSumMax, "random-sum-max", cfg.RandomSumMax, "Maximum random value used by random-sum request source")

	fs.StringVar(&cfg.LoadCalculator, "load-calculator", cfg.LoadCalculator, "Load calculator: step, exponential, logarithmic")
	fs.IntVar(&cfg.MinRPS, "min-rps", cfg.MinRPS, "Minimum requests per second")
	fs.IntVar(&cfg.MaxRPS, "max-rps", cfg.MaxRPS, "Maximum requests per second")
	fs.IntVar(&cfg.StepRPS, "step-rps", cfg.StepRPS, "Step increase for step load calculator")

	fs.DurationVar(&cfg.ScheduleInterval, "schedule-interval", cfg.ScheduleInterval, "How often to dispatch jobs")
	fs.DurationVar(&cfg.JobDuration, "job-duration", cfg.JobDuration, "Duration of each dispatched job")
	fs.DurationVar(&cfg.MetricsPollInterval, "metrics-poll-interval", cfg.MetricsPollInterval, "How often to poll target pod metrics")

	fs.BoolVar(&cfg.InCluster, "in-cluster", cfg.InCluster, "Use in-cluster Kubernetes config")
	fs.StringVar(&cfg.Kubeconfig, "kubeconfig", cfg.Kubeconfig, "Kubeconfig path for out-of-cluster mode")
}

func ParseConfig(args []string) (Config, error) {
	cfg := DefaultConfig()
	fs := flag.NewFlagSet("orchestrator", flag.ContinueOnError)
	BindFlags(fs, &cfg)
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func ValidateConfig(cfg Config) error {
	if cfg.TargetNamespace == "" {
		return fmt.Errorf("target-namespace is required")
	}
	if cfg.RequestSourceType == "" {
		return fmt.Errorf("request-source-type is required")
	}
	if cfg.LoadCalculator == "" {
		return fmt.Errorf("load-calculator is required")
	}
	if cfg.MinRPS < 0 || cfg.MaxRPS < 0 {
		return fmt.Errorf("min-rps and max-rps must be >= 0")
	}
	if cfg.MaxRPS < cfg.MinRPS {
		return fmt.Errorf("max-rps must be >= min-rps")
	}
	if cfg.ScheduleInterval <= 0 {
		return fmt.Errorf("schedule-interval must be > 0")
	}
	if cfg.JobDuration <= 0 {
		return fmt.Errorf("job-duration must be > 0")
	}
	if cfg.MetricsPollInterval <= 0 {
		return fmt.Errorf("metrics-poll-interval must be > 0")
	}
	switch cfg.TargetMode {
	case string(k8s.TargetModePod):
		if cfg.TargetDeployment == "" {
			return fmt.Errorf("target-deployment is required in pod mode")
		}
	case string(k8s.TargetModeService):
		if cfg.TargetService == "" {
			return fmt.Errorf("target-service is required in service mode")
		}
	default:
		return fmt.Errorf("unsupported target-mode %q", cfg.TargetMode)
	}
	return nil
}
