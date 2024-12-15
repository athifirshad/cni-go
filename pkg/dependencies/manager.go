package dependencies

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	ManagerSocket = "/var/run/cni/manager.sock"
)

type Manager struct {
	networkMap *NetworkMap
	depMap     *DependencyMap
	listener   net.Listener
	k8sClient  kubernetes.Interface
}

func NewManager() (*Manager, error) {
	// Create kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	depMap, err := NewDependencyMap()
	if err != nil {
		return nil, err
	}

	return &Manager{
		networkMap: NewNetworkMap(),
		depMap:     depMap,
		k8sClient:  client,
	}, nil
}

func (m *Manager) watchPods() {
	watcher, err := m.k8sClient.CoreV1().Pods("").Watch(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to watch pods: %v", err)
		return
	}
	defer watcher.Stop()

	for event := range watcher.ResultChan() {
		pod, ok := event.Object.(*v1.Pod)
		if !ok {
			continue
		}

		switch event.Type {
		case watch.Added:
			m.handlePodAdd(pod)
		case watch.Deleted:
			m.handlePodDelete(pod)
		}
	}
}

func (m *Manager) handlePodAdd(pod *v1.Pod) {
	m.networkMap.mutex.Lock()
	defer m.networkMap.mutex.Unlock()

	// Create container network info
	containerInfo := &ContainerNetwork{
		ContainerID: string(pod.UID),
		PodName:     pod.Name,
		Namespace:   pod.Namespace,
		Labels:      pod.Labels,
		IPAddress:   pod.Status.PodIP,
		Interface:   "eth0", // Default interface name
	}

	// Add to network map
	m.networkMap.containers[string(pod.UID)] = containerInfo

	// Update eBPF maps if needed
	if err := m.updateBPFMaps(); err != nil {
		log.Printf("Failed to update BPF maps for pod %s/%s: %v",
			pod.Namespace, pod.Name, err)
	}

	log.Printf("Added pod %s/%s to network map", pod.Namespace, pod.Name)
}

func (m *Manager) handlePodDelete(pod *v1.Pod) {
	m.networkMap.mutex.Lock()
	defer m.networkMap.mutex.Unlock()

	delete(m.networkMap.containers, string(pod.UID))
}

func (m *Manager) serve() error {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			return err
		}
		go m.handleConnection(conn)
	}
}

func (m *Manager) handleConnection(conn net.Conn) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	var req CNIRequest
	if err := decoder.Decode(&req); err != nil {
		log.Printf("Failed to decode request: %v", err)
		return
	}

	switch req.Command {
	case "ADD":
		m.handleAdd(&req)
	case "DEL":
		m.handleDel(&req)
	}
}

type CNIRequest struct {
	Command     string `json:"command"`
	ContainerID string `json:"container_id"`
	Netns       string `json:"netns"`
	IfName      string `json:"ifname"`
}

func (m *Manager) handleAdd(req *CNIRequest) error {
	m.networkMap.mutex.Lock()
	defer m.networkMap.mutex.Unlock()

	// Create container network info
	containerInfo := &ContainerNetwork{
		ContainerID: req.ContainerID,
		Interface:   req.IfName,
		// Other fields will be populated by pod watcher
	}

	m.networkMap.containers[req.ContainerID] = containerInfo
	return m.updateBPFMaps()
}

func (m *Manager) handleDel(req *CNIRequest) error {
	m.networkMap.mutex.Lock()
	defer m.networkMap.mutex.Unlock()

	delete(m.networkMap.containers, req.ContainerID)
	return m.updateBPFMaps()
}

func (m *Manager) Start() error {
	// Start Unix socket listener
	if err := os.MkdirAll(filepath.Dir(ManagerSocket), 0755); err != nil {
		return err
	}

	l, err := net.Listen("unix", ManagerSocket)
	if err != nil {
		return err
	}
	m.listener = l

	// Watch K8s events
	go m.watchPods()

	// Handle CNI requests
	return m.serve()
}

func (m *Manager) Cleanup() error {
	if m.listener != nil {
		m.listener.Close()
	}

	if m.depMap != nil && m.depMap.Path != "" {
		if err := os.Remove(m.depMap.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove BPF map: %v", err)
		}
	}
	return nil
}
