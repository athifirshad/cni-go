package store

import (
	"fmt"
	"strings"
	"sync"

	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"github.com/athifirshad/go-cni/pkg/logging"

)

var logger = logging.NewLogger("store")

type NetworkStore struct {
	mu         sync.RWMutex
	pods       map[string]*v1.Pod                     // key: namespace/name
	policies   map[string]*networkingv1.NetworkPolicy // key: namespace/name
	containers map[string]*ContainerInfo              // key: containerID
}

type ContainerInfo struct {
	ID        string
	Namespace string
	PodName   string
	NetNS     string
	IfName    string
}

var DefaultStore = NewNetworkStore()

func NewNetworkStore() *NetworkStore {
	return &NetworkStore{
		pods:       make(map[string]*v1.Pod),
		policies:   make(map[string]*networkingv1.NetworkPolicy),
		containers: make(map[string]*ContainerInfo),
	}
}

func (s *NetworkStore) AddPod(pod *v1.Pod) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := pod.Namespace + "/" + pod.Name
	s.pods[key] = pod
}

func (s *NetworkStore) DeletePod(namespace, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pods, namespace+"/"+name)
}

func (s *NetworkStore) UpdatePolicy(policy *networkingv1.NetworkPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := policy.Namespace + "/" + policy.Name
	s.policies[key] = policy
}

func (s *NetworkStore) DeletePolicy(namespace, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.policies, namespace+"/"+name)
}

func (s *NetworkStore) Reconcile(pods *v1.PodList, policies *networkingv1.NetworkPolicyList) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear existing state
	s.pods = make(map[string]*v1.Pod)
	s.policies = make(map[string]*networkingv1.NetworkPolicy)

	// Add current pods
	for i := range pods.Items {
		pod := &pods.Items[i]
		key := pod.Namespace + "/" + pod.Name
		s.pods[key] = pod
	}

	// Add current policies
	for i := range policies.Items {
		policy := &policies.Items[i]
		key := policy.Namespace + "/" + policy.Name
		s.policies[key] = policy
	}
}

func (s *NetworkStore) AddContainer(info *ContainerInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.containers[info.ID] = info
	logger.Info("Added container %s to store", info.ID)
}

func (s *NetworkStore) DeleteContainer(containerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.containers, containerID)
	logger.Info("Removed container %s from store", containerID)
}

func (s *NetworkStore) GetContainer(containerID string) (*ContainerInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	info, exists := s.containers[containerID]
	return info, exists
}

func (s *NetworkStore) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var b strings.Builder
	b.WriteString("\n=== Network Store State ===\n")

	b.WriteString("\nPods:\n")
	for k, pod := range s.pods {
		b.WriteString(fmt.Sprintf("  %s:\n    Phase: %s\n    IP: %s\n",
			k, pod.Status.Phase, pod.Status.PodIP))
	}

	b.WriteString("\nNetwork Policies:\n")
	for k, policy := range s.policies {
		b.WriteString(fmt.Sprintf("  %s:\n    Ingress Rules: %d\n    Egress Rules: %d\n",
			k, len(policy.Spec.Ingress), len(policy.Spec.Egress)))
	}

	b.WriteString("\nContainers:\n")
	for id, info := range s.containers {
		b.WriteString(fmt.Sprintf("  %s:\n    Pod: %s/%s\n    NetNS: %s\n    Interface: %s\n",
			id, info.Namespace, info.PodName, info.NetNS, info.IfName))
	}

	return b.String()
}
