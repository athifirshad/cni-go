package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/athifirshad/go-cni/pkg/logging"
	"github.com/athifirshad/go-cni/pkg/store"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var logger = logging.NewLogger("manager")

type NetworkManager struct {
	client *kubernetes.Clientset
	store  *store.NetworkStore
}

func newNetworkManager() (*NetworkManager, error) {
	logger.Info("Initializing network manager")

	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Debug("Failed to get in-cluster config, falling back to kubeconfig: %v", err)
		return nil, fmt.Errorf("must run in cluster, in-cluster config required: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	logger.Info("Successfully initialized Kubernetes client")
	return &NetworkManager{
		client: clientset,
		store:  store.DefaultStore,
	}, nil
}

// Add wait function
func waitForAPIServer(client *kubernetes.Clientset) error {
	logger.Info("Waiting for API server to be ready")
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for API server")
		case <-ticker.C:
			_, err := client.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
			if err == nil {
				logger.Info("Successfully connected to API server")
				return nil
			}
			logger.Debug("API server not ready yet: %v", err)
		}
	}
}

func (m *NetworkManager) watchWithRetry(ctx context.Context, watch func(context.Context) error) {
	backoff := time.Second
	maxBackoff := time.Minute
	for {
		if err := watch(ctx); err != nil {
			logger.Error("Watch failed: %v, retrying in %v", err, backoff)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		backoff = time.Second
	}
}

func (m *NetworkManager) watchPods(ctx context.Context) error {
	logger.Info("Starting pod watcher")

	watcher, err := m.client.CoreV1().Pods("").Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to watch pods: %v", err)
	}

	for event := range watcher.ResultChan() {
		pod, ok := event.Object.(*v1.Pod)
		if !ok {
			continue
		}

		switch event.Type {
		case watch.Added:
			logger.Info("Pod added to store: %s/%s", pod.Namespace, pod.Name)
			m.store.AddPod(pod)
		case watch.Modified:
			logger.Info("Pod updated in store: %s/%s", pod.Namespace, pod.Name)
			m.store.AddPod(pod) // Updates existing entry
		case watch.Deleted:
			logger.Info("Pod removed from store: %s/%s", pod.Namespace, pod.Name)
			m.store.DeletePod(pod.Namespace, pod.Name)
		}
	}
	return nil
}

func (m *NetworkManager) watchNetworkPolicies(ctx context.Context) error {
	logger.Info("Starting network policy watcher")

	watcher, err := m.client.NetworkingV1().NetworkPolicies("").Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to watch network policies: %v", err)
	}

	for event := range watcher.ResultChan() {
		policy, ok := event.Object.(*networkingv1.NetworkPolicy)
		if !ok {
			continue
		}

		switch event.Type {
		case watch.Added, watch.Modified:
			logger.Info("Network policy updated: %s/%s", policy.Namespace, policy.Name)
			// Update store with new policy
			m.store.UpdatePolicy(policy)
		case watch.Deleted:
			logger.Info("Network policy deleted: %s/%s", policy.Namespace, policy.Name)
			// Remove network policy from store
			m.store.DeletePolicy(policy.Namespace, policy.Name)
		}
	}
	return nil
}

func (m *NetworkManager) reconcileNetworks(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logger.Info("Starting network reconciliation")

			// Get all pods
			pods, err := m.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
			if err != nil {
				logger.Error("Failed to list pods: %v", err)
				continue
			}

			// Get all network policies
			policies, err := m.client.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
			if err != nil {
				logger.Error("Failed to list network policies: %v", err)
				continue
			}

			// Reconcile store with current state
			m.store.Reconcile(pods, policies)
			logger.Info("Store reconciled with %d pods and %d policies", 
                len(pods.Items), len(policies.Items))
		}
	}
}

func main() {
	logger.Info("Starting CNI manager")

	manager, err := newNetworkManager()
	if err != nil {
		logger.Error("Failed to create network manager: %v", err)
		os.Exit(1)
	}

	if err := waitForAPIServer(manager.client); err != nil {
		logger.Error("Failed to connect to API server: %v", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Wrap watchers with retry logic
	go manager.watchWithRetry(ctx, manager.watchPods)
	go manager.watchWithRetry(ctx, manager.watchNetworkPolicies)
	go manager.reconcileNetworks(ctx)

	logger.Info("CNI manager initialized successfully")
	// Block until context is cancelled
	<-ctx.Done()
	logger.Info("Shutting down CNI manager")

}
