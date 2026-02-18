package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/PeladoCollado/imager/metrics"
	"github.com/PeladoCollado/imager/orchestrator/api"
	"github.com/PeladoCollado/imager/orchestrator/k8s"
	"github.com/PeladoCollado/imager/orchestrator/logger"
	"github.com/PeladoCollado/imager/orchestrator/manager"
	"github.com/PeladoCollado/imager/orchestrator/requests"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type config struct {
	ListenPort int

	TargetMode       string
	TargetNamespace  string
	TargetDeployment string
	TargetService    string
	TargetPortName   string
	TargetScheme     string

	RequestSourceFile string

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

func main() {
	cfg := parseConfig()
	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	kubeConfig, err := initKubeConfig(cfg)
	if err != nil {
		logger.Logger.Error("Unable to initialize Kubernetes configuration", err)
		os.Exit(1)
	}
	kubeClient, err := k8s.NewClient(kubeConfig)
	if err != nil {
		logger.Logger.Error("Unable to initialize Kubernetes clients", err)
		os.Exit(1)
	}

	source, err := requests.NewFileReader(cfg.RequestSourceFile)
	if err != nil {
		logger.Logger.Error("Unable to initialize request source", err)
		os.Exit(1)
	}

	calculator, err := newLoadCalculator(cfg)
	if err != nil {
		logger.Logger.Error("Unable to initialize load calculator", err)
		os.Exit(1)
	}

	resolverConfig := k8s.TargetResolverConfig{
		Mode:       k8s.TargetMode(cfg.TargetMode),
		Namespace:  cfg.TargetNamespace,
		Deployment: cfg.TargetDeployment,
		Service:    cfg.TargetService,
		PortName:   cfg.TargetPortName,
		Scheme:     cfg.TargetScheme,
	}
	targetResolver, err := k8s.NewTargetResolver(kubeClient, resolverConfig)
	if err != nil {
		logger.Logger.Error("Unable to initialize target resolver", err)
		os.Exit(1)
	}

	orchestratorMetrics := metrics.NewOrchestratorMetrics(prometheus.DefaultRegisterer)
	manager.Schedule(
		ctx,
		calculator,
		source,
		targetResolver,
		orchestratorMetrics,
		manager.ScheduleOptions{
			Interval:    cfg.ScheduleInterval,
			JobDuration: cfg.JobDuration,
		},
	)

	go pollPodMetrics(ctx, targetResolver, kubeClient, cfg.TargetNamespace, orchestratorMetrics, cfg.MetricsPollInterval)

	baseHandler := api.NewHandler(ctx)
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/", baseHandler)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.ListenPort),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Logger.Warn("Unable to gracefully shutdown orchestrator server", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Logger.Error("Orchestrator server failed", err)
		os.Exit(1)
	}
}

func parseConfig() config {
	cfg := config{}
	flag.IntVar(&cfg.ListenPort, "listen-port", 8099, "Orchestrator API and metrics port")

	flag.StringVar(&cfg.TargetMode, "target-mode", string(k8s.TargetModePod), "Target mode: pod or service")
	flag.StringVar(&cfg.TargetNamespace, "target-namespace", "default", "Kubernetes namespace for the target")
	flag.StringVar(&cfg.TargetDeployment, "target-deployment", "", "Target deployment name (pod mode)")
	flag.StringVar(&cfg.TargetService, "target-service", "", "Target service name (service mode)")
	flag.StringVar(&cfg.TargetPortName, "target-port-name", "http", "Target container port name")
	flag.StringVar(&cfg.TargetScheme, "target-scheme", "http", "Target request URL scheme")

	flag.StringVar(&cfg.RequestSourceFile, "request-source-file", "/config/requests.json", "Path to request source JSON file")

	flag.StringVar(&cfg.LoadCalculator, "load-calculator", "step", "Load calculator: step, exponential, logarithmic")
	flag.IntVar(&cfg.MinRPS, "min-rps", 1, "Minimum requests per second")
	flag.IntVar(&cfg.MaxRPS, "max-rps", 100, "Maximum requests per second")
	flag.IntVar(&cfg.StepRPS, "step-rps", 1, "Step increase for step load calculator")

	flag.DurationVar(&cfg.ScheduleInterval, "schedule-interval", time.Second, "How often to dispatch jobs")
	flag.DurationVar(&cfg.JobDuration, "job-duration", time.Second, "Duration of each dispatched job")
	flag.DurationVar(&cfg.MetricsPollInterval, "metrics-poll-interval", 5*time.Second, "How often to poll target pod metrics")

	flag.BoolVar(&cfg.InCluster, "in-cluster", true, "Use in-cluster Kubernetes config")
	flag.StringVar(&cfg.Kubeconfig, "kubeconfig", "", "Kubeconfig path for out-of-cluster mode")
	flag.Parse()
	return cfg
}

func validateConfig(cfg config) error {
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
	if cfg.RequestSourceFile == "" {
		return fmt.Errorf("request-source-file is required")
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

func initKubeConfig(cfg config) (*rest.Config, error) {
	if cfg.InCluster {
		return k8s.InitInCluster()
	}
	return k8s.InitOffCluster(cfg.Kubeconfig)
}

func newLoadCalculator(cfg config) (manager.LoadCalculator, error) {
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

func pollPodMetrics(ctx context.Context,
	resolver *k8s.TargetResolver,
	client *k8s.Client,
	namespace string,
	metricsCollector *metrics.OrchestratorMetrics,
	interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pods, err := resolver.CurrentPods(ctx)
			if err != nil {
				logger.Logger.Warn("Unable to resolve current pods for metrics collection", err)
				continue
			}
			podNames := podNames(pods)
			if len(podNames) == 0 {
				metricsCollector.ResetTargetPodUsage()
				continue
			}
			usageMap, err := client.PodResourceUsage(ctx, namespace, podNames)
			if err != nil {
				logger.Logger.Warn("Unable to collect pod resource metrics", err)
				continue
			}
			metricsCollector.ResetTargetPodUsage()
			for podName, usage := range usageMap {
				metricsCollector.SetTargetPodUsage(namespace, podName, usage.CPUMillicores, usage.MemoryBytes)
			}
		}
	}
}

func podNames(pods []v1.Pod) []string {
	names := make([]string, 0, len(pods))
	for _, pod := range pods {
		names = append(names, pod.Name)
	}
	return names
}
