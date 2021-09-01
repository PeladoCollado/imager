package k8s

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
	"k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
	"path/filepath"
)

type BasePathConfig map[string]string

type BasePathSupplier interface {
	BuildPath(config BasePathConfig) string
}

func NewPodClient(ctx context.Context, config *rest.Config, namespace string, label string, container string, portname string) (*PodClient, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	pod, err := selectPod(ctx, clientset, namespace, label)
	if err != nil {
		return nil, err
	}

	metricsClient, err := metrics.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	metricses := metricsClient.MetricsV1beta1().PodMetricses(namespace)

	return &PodClient{
		ctx:       ctx,
		namespace: namespace,
		label:     label,
		container: container,
		portname:  portname,
		pod:       pod,
		m:         metricses,
		clientset: clientset,
	}, nil
}

func selectPod(ctx context.Context, clientset *kubernetes.Clientset, namespace string, label string) (v1.Pod, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: label, Limit: 100})
	if err != nil {
		return v1.Pod{}, err
	}
	count := len(pods.Items)
	itm := rand.IntnRange(0, count)
	return pods.Items[itm], nil
}

type PodClient struct {
	ctx       context.Context
	namespace string
	label     string
	container string
	portname  string
	pod       v1.Pod
	m         v1beta1.PodMetricsInterface
	clientset *kubernetes.Clientset
}

func (pc *PodClient) BuildPath(conf BasePathConfig) string {
	ip := pc.pod.Status.PodIP

	var port int32

portsearch:
	for _, c := range pc.pod.Spec.Containers {
		for _, p := range c.Ports {
			if p.Name == pc.portname {
				port = p.ContainerPort
				break portsearch
			}
		}
	}
	return fmt.Sprintf("http://%s:%d/", ip, port)
}

func (pc *PodClient) GetMetrics() (map[string]interface{}, error) {
	podMetrics, err := pc.m.Get(pc.ctx, pc.pod.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	metricsMap := make(map[string]interface{})
	for _, c := range podMetrics.Containers {
		if c.Name == pc.container {
			for k, v := range c.Usage {
				i64, b := v.AsInt64()
				if b {
					metricsMap[string(k)] = i64
				} else {
					metricsMap[string(k)] = v.AsDec()
				}
			}
		}
	}
	return metricsMap, nil
}

func InitInCluster() (*rest.Config, error) {
	return rest.InClusterConfig()
}

func InitOffCluster() (*rest.Config, error) {
	home := homedir.HomeDir()
	path := filepath.Join(home, ".kube", "config")
	return clientcmd.BuildConfigFromFlags("", path)
}
