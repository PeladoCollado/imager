package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

func TestPodsForDeploymentReturnsRunningPods(t *testing.T) {
	namespace := "default"
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "target"}},
		},
	}
	runningPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-running", Namespace: namespace, Labels: map[string]string{"app": "target"}},
		Status:     v1.PodStatus{Phase: v1.PodRunning, PodIP: "10.0.0.1"},
	}
	pendingPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-pending", Namespace: namespace, Labels: map[string]string{"app": "target"}},
		Status:     v1.PodStatus{Phase: v1.PodPending, PodIP: "10.0.0.2"},
	}
	kubeClient := k8sfake.NewSimpleClientset(deployment, &runningPod, &pendingPod)
	metricsClient := metricsfake.NewSimpleClientset()
	client := NewClientWithClients(kubeClient, metricsClient)

	pods, err := client.PodsForDeployment(context.Background(), namespace, "target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pods) != 1 || pods[0].Name != "pod-running" {
		t.Fatalf("expected only running pod, got %+v", pods)
	}
}

func TestPodsForServiceReturnsRunningPods(t *testing.T) {
	namespace := "default"
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "target-svc", Namespace: namespace},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "svc-target"},
		},
	}
	runningPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-running", Namespace: namespace, Labels: map[string]string{"app": "svc-target"}},
		Status:     v1.PodStatus{Phase: v1.PodRunning, PodIP: "10.0.0.1"},
	}
	succeededPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-done", Namespace: namespace, Labels: map[string]string{"app": "svc-target"}},
		Status:     v1.PodStatus{Phase: v1.PodSucceeded, PodIP: "10.0.0.2"},
	}
	kubeClient := k8sfake.NewSimpleClientset(service, &runningPod, &succeededPod)
	metricsClient := metricsfake.NewSimpleClientset()
	client := NewClientWithClients(kubeClient, metricsClient)

	pods, err := client.PodsForService(context.Background(), namespace, "target-svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pods) != 1 || pods[0].Name != "pod-running" {
		t.Fatalf("expected only running pod, got %+v", pods)
	}
}

func TestBuildTargetURLs(t *testing.T) {
	pods := []v1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-2"},
			Status:     v1.PodStatus{PodIP: "10.0.0.2"},
			Spec: v1.PodSpec{
				Containers: []v1.Container{{Ports: []v1.ContainerPort{{Name: "http", ContainerPort: 8080}}}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-1"},
			Status:     v1.PodStatus{PodIP: "10.0.0.1"},
			Spec: v1.PodSpec{
				Containers: []v1.Container{{Ports: []v1.ContainerPort{{Name: "http", ContainerPort: 8080}}}},
			},
		},
	}

	urls := BuildTargetURLs(pods, "http", "http")
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d", len(urls))
	}
	if urls[0] != "http://10.0.0.1:8080" || urls[1] != "http://10.0.0.2:8080" {
		t.Fatalf("unexpected sorted URLs: %+v", urls)
	}
}

func TestPodResourceUsage(t *testing.T) {
	namespace := "default"
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "target-pod", Namespace: namespace},
		Status:     v1.PodStatus{Phase: v1.PodRunning, PodIP: "10.0.0.10"},
	}
	kubeClient := k8sfake.NewSimpleClientset(&pod)

	podMetrics := &metricsv1beta1.PodMetrics{
		ObjectMeta: metav1.ObjectMeta{Name: "target-pod", Namespace: namespace},
		Containers: []metricsv1beta1.ContainerMetrics{
			{
				Name: "app",
				Usage: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("250m"),
					v1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
		},
	}
	metricsClient := metricsfake.NewSimpleClientset()
	if err := metricsClient.Tracker().Create(metricsv1beta1.SchemeGroupVersion.WithResource("pods"), podMetrics, namespace); err != nil {
		t.Fatalf("unable to seed metrics tracker: %v", err)
	}
	client := NewClientWithClients(kubeClient, metricsClient)

	usage, err := client.PodResourceUsage(context.Background(), namespace, []string{"target-pod"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := usage["target-pod"]
	if !ok {
		t.Fatalf("expected usage entry for target-pod")
	}
	if got.CPUMillicores != 250 {
		t.Fatalf("expected 250 millicores, got %d", got.CPUMillicores)
	}
	if got.MemoryBytes <= 0 {
		t.Fatalf("expected positive memory bytes, got %d", got.MemoryBytes)
	}
}

func TestTargetResolverServiceMode(t *testing.T) {
	namespace := "default"
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "target-svc", Namespace: namespace},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "svc-target"},
			Ports: []v1.ServicePort{
				{Name: "http", Port: 8080},
			},
		},
	}
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-running", Namespace: namespace, Labels: map[string]string{"app": "svc-target"}},
		Status:     v1.PodStatus{Phase: v1.PodRunning, PodIP: "10.0.0.1"},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{Ports: []v1.ContainerPort{{Name: "http", ContainerPort: 8080}}}},
		},
	}
	kubeClient := k8sfake.NewSimpleClientset(service, &pod)
	metricsClient := metricsfake.NewSimpleClientset()
	client := NewClientWithClients(kubeClient, metricsClient)

	resolver, err := NewTargetResolver(client, TargetResolverConfig{
		Mode:      TargetModeService,
		Namespace: namespace,
		Service:   "target-svc",
		PortName:  "http",
		Scheme:    "http",
	})
	if err != nil {
		t.Fatalf("unexpected resolver init error: %v", err)
	}

	targets, err := resolver.ResolveTargets(context.Background())
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}
	if len(targets) != 1 || targets[0] != "http://target-svc.default.svc:8080" {
		t.Fatalf("unexpected targets: %+v", targets)
	}

	pods, err := resolver.CurrentPods(context.Background())
	if err != nil {
		t.Fatalf("unexpected current pods error: %v", err)
	}
	if len(pods) != 1 || pods[0].Name != "pod-running" {
		t.Fatalf("unexpected resolved pods for metrics: %+v", pods)
	}
}

func TestServiceTargetURL(t *testing.T) {
	namespace := "default"
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "target-svc", Namespace: namespace},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{Name: "http", Port: 8080},
			},
		},
	}
	kubeClient := k8sfake.NewSimpleClientset(service)
	client := NewClientWithClients(kubeClient, metricsfake.NewSimpleClientset())

	targetURL, err := client.ServiceTargetURL(context.Background(), namespace, "target-svc", "http", "https")
	if err != nil {
		t.Fatalf("unexpected error resolving service target URL: %v", err)
	}
	if targetURL != "https://target-svc.default.svc:8080" {
		t.Fatalf("unexpected service target URL: %s", targetURL)
	}
}

func TestTargetResolverURLMode(t *testing.T) {
	resolver, err := NewTargetResolver(nil, TargetResolverConfig{
		Mode: TargetModeURL,
		URL:  "https://example.com:8443",
	})
	if err != nil {
		t.Fatalf("unexpected resolver init error: %v", err)
	}

	targets, err := resolver.ResolveTargets(context.Background())
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}
	if len(targets) != 1 || targets[0] != "https://example.com:8443" {
		t.Fatalf("unexpected targets: %+v", targets)
	}

	pods, err := resolver.CurrentPods(context.Background())
	if err != nil {
		t.Fatalf("unexpected current pods error: %v", err)
	}
	if len(pods) != 0 {
		t.Fatalf("expected no pods in url mode, got %d", len(pods))
	}
}

func TestTargetResolverURLModeRejectsInvalidURL(t *testing.T) {
	if _, err := NewTargetResolver(nil, TargetResolverConfig{
		Mode: TargetModeURL,
		URL:  "example.com/no-scheme",
	}); err == nil {
		t.Fatalf("expected invalid url init error")
	}
}
