package k8s

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"path/filepath"
	"sort"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

type TargetMode string

const (
	TargetModePod     TargetMode = "pod"
	TargetModeService TargetMode = "service"
	TargetModeURL     TargetMode = "url"
)

type PodUsage struct {
	CPUMillicores int64
	MemoryBytes   int64
}

type Client struct {
	kubeClient    kubernetes.Interface
	metricsClient metricsclient.Interface
}

func NewClient(config *rest.Config) (*Client, error) {
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	metricsAPI, err := metricsclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Client{kubeClient: kubeClient, metricsClient: metricsAPI}, nil
}

func NewClientWithClients(kubeClient kubernetes.Interface, metricsAPI metricsclient.Interface) *Client {
	return &Client{kubeClient: kubeClient, metricsClient: metricsAPI}
}

func (c *Client) PodsForDeployment(ctx context.Context, namespace string, deploymentName string) ([]v1.Pod, error) {
	deployment, err := c.kubeClient.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	selector, err := selectorForDeployment(deployment)
	if err != nil {
		return nil, err
	}
	pods, err := c.kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	return runningPods(pods.Items), nil
}

func (c *Client) PodsForService(ctx context.Context, namespace string, serviceName string) ([]v1.Pod, error) {
	service, err := c.kubeClient.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if len(service.Spec.Selector) == 0 {
		return nil, fmt.Errorf("service %s/%s has no selector labels", namespace, serviceName)
	}
	selector := labels.SelectorFromSet(service.Spec.Selector)
	pods, err := c.kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	return runningPods(pods.Items), nil
}

func (c *Client) PodResourceUsage(ctx context.Context, namespace string, podNames []string) (map[string]PodUsage, error) {
	usageByPod := make(map[string]PodUsage, len(podNames))
	for _, podName := range podNames {
		metrics, err := c.metricsClient.MetricsV1beta1().PodMetricses(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		var cpuMilli int64
		var memoryBytes int64
		for _, container := range metrics.Containers {
			cpuMilli += container.Usage.Cpu().MilliValue()
			memoryBytes += container.Usage.Memory().Value()
		}
		usageByPod[podName] = PodUsage{
			CPUMillicores: cpuMilli,
			MemoryBytes:   memoryBytes,
		}
	}
	return usageByPod, nil
}

func SelectRandomPod(pods []v1.Pod) (*v1.Pod, error) {
	if len(pods) == 0 {
		return nil, fmt.Errorf("no pods available")
	}
	index := rand.Intn(len(pods))
	return &pods[index], nil
}

func BuildTargetURLs(pods []v1.Pod, portName string, scheme string) []string {
	if scheme == "" {
		scheme = "http"
	}
	urls := make([]string, 0, len(pods))
	for _, pod := range pods {
		if pod.Status.PodIP == "" {
			continue
		}
		port := findPort(pod, portName)
		urls = append(urls, fmt.Sprintf("%s://%s:%d", scheme, pod.Status.PodIP, port))
	}
	sort.Strings(urls)
	return urls
}

type TargetResolverConfig struct {
	Mode       TargetMode
	Namespace  string
	Deployment string
	Service    string
	URL        string
	PortName   string
	Scheme     string
}

type TargetResolver struct {
	client *Client
	cfg    TargetResolverConfig

	lock     sync.RWMutex
	lastPods []v1.Pod
}

func NewTargetResolver(client *Client, cfg TargetResolverConfig) (*TargetResolver, error) {
	switch cfg.Mode {
	case TargetModePod:
		if cfg.Namespace == "" {
			return nil, fmt.Errorf("namespace is required in pod mode")
		}
		if client == nil {
			return nil, fmt.Errorf("kubernetes client is required in pod mode")
		}
		if cfg.Deployment == "" {
			return nil, fmt.Errorf("deployment is required in pod mode")
		}
	case TargetModeService:
		if cfg.Namespace == "" {
			return nil, fmt.Errorf("namespace is required in service mode")
		}
		if client == nil {
			return nil, fmt.Errorf("kubernetes client is required in service mode")
		}
		if cfg.Service == "" {
			return nil, fmt.Errorf("service is required in service mode")
		}
	case TargetModeURL:
		if cfg.URL == "" {
			return nil, fmt.Errorf("url is required in url mode")
		}
		parsed, err := url.Parse(cfg.URL)
		if err != nil || !parsed.IsAbs() {
			return nil, fmt.Errorf("url must be absolute in url mode")
		}
	default:
		return nil, fmt.Errorf("unsupported target mode %q", cfg.Mode)
	}
	return &TargetResolver{
		client: client,
		cfg:    cfg,
	}, nil
}

func (r *TargetResolver) ResolveTargets(ctx context.Context) ([]string, error) {
	if r.cfg.Mode == TargetModeURL {
		return []string{r.cfg.URL}, nil
	}

	pods, err := r.resolvePods(ctx)
	if err != nil {
		return nil, err
	}
	r.lock.Lock()
	r.lastPods = append([]v1.Pod(nil), pods...)
	r.lock.Unlock()
	if r.cfg.Mode == TargetModeService {
		targetURL, urlErr := r.client.ServiceTargetURL(ctx, r.cfg.Namespace, r.cfg.Service, r.cfg.PortName, r.cfg.Scheme)
		if urlErr != nil {
			return nil, urlErr
		}
		return []string{targetURL}, nil
	}
	return BuildTargetURLs(pods, r.cfg.PortName, r.cfg.Scheme), nil
}

func (r *TargetResolver) CurrentPods(ctx context.Context) ([]v1.Pod, error) {
	r.lock.RLock()
	if len(r.lastPods) > 0 {
		pods := append([]v1.Pod(nil), r.lastPods...)
		r.lock.RUnlock()
		return pods, nil
	}
	r.lock.RUnlock()

	pods, err := r.resolvePods(ctx)
	if err != nil {
		return nil, err
	}
	r.lock.Lock()
	r.lastPods = append([]v1.Pod(nil), pods...)
	r.lock.Unlock()
	return pods, nil
}

func (r *TargetResolver) resolvePods(ctx context.Context) ([]v1.Pod, error) {
	switch r.cfg.Mode {
	case TargetModePod:
		pods, err := r.client.PodsForDeployment(ctx, r.cfg.Namespace, r.cfg.Deployment)
		if err != nil {
			return nil, err
		}
		selectedPod, err := SelectRandomPod(pods)
		if err != nil {
			return nil, err
		}
		return []v1.Pod{*selectedPod}, nil
	case TargetModeService:
		return r.client.PodsForService(ctx, r.cfg.Namespace, r.cfg.Service)
	case TargetModeURL:
		return []v1.Pod{}, nil
	default:
		return nil, fmt.Errorf("unsupported target mode %q", r.cfg.Mode)
	}
}

func selectorForDeployment(deployment *appsv1.Deployment) (labels.Selector, error) {
	if deployment.Spec.Selector == nil {
		return nil, fmt.Errorf("deployment %s has nil selector", deployment.Name)
	}
	return metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
}

func runningPods(pods []v1.Pod) []v1.Pod {
	result := make([]v1.Pod, 0, len(pods))
	for _, pod := range pods {
		if pod.Status.Phase == v1.PodRunning {
			result = append(result, pod)
		}
	}
	return result
}

func findPort(pod v1.Pod, portName string) int32 {
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if portName == "" || port.Name == portName {
				return port.ContainerPort
			}
		}
	}
	return 80
}

func (c *Client) ServiceTargetURL(ctx context.Context,
	namespace string,
	serviceName string,
	portName string,
	scheme string) (string, error) {
	service, err := c.kubeClient.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	if scheme == "" {
		scheme = "http"
	}
	port := findServicePort(*service, portName)
	host := fmt.Sprintf("%s.%s.svc", service.Name, namespace)
	return fmt.Sprintf("%s://%s:%d", scheme, host, port), nil
}

func findServicePort(service v1.Service, portName string) int32 {
	for _, port := range service.Spec.Ports {
		if portName == "" || port.Name == portName {
			return port.Port
		}
	}
	if len(service.Spec.Ports) > 0 {
		return service.Spec.Ports[0].Port
	}
	return 80
}

func InitInCluster() (*rest.Config, error) {
	return rest.InClusterConfig()
}

func InitOffCluster(kubeconfigPath string) (*rest.Config, error) {
	path := kubeconfigPath
	if path == "" {
		home := homedir.HomeDir()
		path = filepath.Join(home, ".kube", "config")
	}
	return clientcmd.BuildConfigFromFlags("", path)
}
