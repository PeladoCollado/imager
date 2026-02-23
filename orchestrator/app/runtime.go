package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/PeladoCollado/imager/metrics"
	"github.com/PeladoCollado/imager/orchestrator/api"
	"github.com/PeladoCollado/imager/orchestrator/k8s"
	"github.com/PeladoCollado/imager/orchestrator/logger"
	"github.com/PeladoCollado/imager/orchestrator/manager"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
)

type RunOptions struct {
	RequestSourceFactory  RequestSourceFactory
	LoadCalculatorFactory LoadCalculatorFactory

	Registerer prometheus.Registerer
	Gatherer   prometheus.Gatherer
}

func Run(ctx context.Context, cfg Config, opts RunOptions) error {
	if err := ValidateConfig(cfg); err != nil {
		return err
	}

	kubeConfig, err := initKubeConfig(cfg)
	if err != nil {
		return fmt.Errorf("initialize kubernetes config: %w", err)
	}

	kubeClient, err := k8s.NewClient(kubeConfig)
	if err != nil {
		return fmt.Errorf("initialize kubernetes clients: %w", err)
	}

	sourceFactory := requestSourceFactoryOrDefault(opts.RequestSourceFactory)
	source, err := sourceFactory.NewRequestSource(cfg)
	if err != nil {
		return fmt.Errorf("initialize request source: %w", err)
	}

	loadFactory := loadCalculatorFactoryOrDefault(opts.LoadCalculatorFactory)
	calculator, err := loadFactory.NewLoadCalculator(cfg)
	if err != nil {
		return fmt.Errorf("initialize load calculator: %w", err)
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
		return fmt.Errorf("initialize target resolver: %w", err)
	}

	registerer := opts.Registerer
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	gatherer := opts.Gatherer
	if gatherer == nil {
		gatherer = prometheus.DefaultGatherer
	}

	orchestratorMetrics := metrics.NewOrchestratorMetrics(registerer)
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
	mux.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))
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
		return fmt.Errorf("orchestrator server failed: %w", err)
	}

	return nil
}

func initKubeConfig(cfg Config) (*rest.Config, error) {
	if cfg.InCluster {
		return k8s.InitInCluster()
	}
	return k8s.InitOffCluster(cfg.Kubeconfig)
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
