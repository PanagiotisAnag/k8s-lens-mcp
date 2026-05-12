package k8s

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned"

	"path/filepath"
)

type Client struct {
	Kube    kubernetes.Interface
	Metrics metricsv1beta1.Interface
}

func NewClient() (*Client, error) {
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	kube, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	metrics, err := metricsv1beta1.NewForConfig(config)
	if err != nil {
		// metrics-server may not be installed; non-fatal
		metrics = nil
	}

	return &Client{Kube: kube, Metrics: metrics}, nil
}
