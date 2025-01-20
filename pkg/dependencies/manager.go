package dependencies

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

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
	log.Println("Starting NewManager...")
	// Create kubernetes client
	log.Println("Creating kubernetes client config...")
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Failed to create kubernetes client config: %v", err)
		return nil, err
	}

	log.Println("Creating kubernetes client...")
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Failed to create kubernetes client: %v", err)
		return nil, err
	}

	depMap, err := NewDependencyMap()
	if err != nil {
		return nil, fmt.Errorf("failed to create DependencyMap: %v", err)
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
		Interface:   "eth0", // TODO: find actual interface name
	}

	// Add to network map
	m.networkMap.containers[string(pod.UID)] = containerInfo

	// Update eBPF maps if needed
	if err := m.updateBPFMaps(); err != nil {
		log.Printf("Failed to update BPF maps for pod %s/%s: %v",
			pod.Namespace, pod.Name, err)
	} else {
		log.Printf("Updated BPF map for pod %s/%s with policy: %v",
			pod.Namespace, pod.Name, pod.Labels["network.policy"])
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

func (m *Manager) waitForPinnedMap() {
	log.Println("Waiting for pinned map to become available...")
	mapPath := "/sys/fs/bpf/container_deps"
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(mapPath); err == nil {
			log.Println("Pinned map found.")
			return
		}
		time.Sleep(2 * time.Second)
	}
	log.Printf("Warning: pinned map %s not found yet.", mapPath)
}

func (m *Manager) syncPods() {
	log.Println("Syncing existing pods...")
	pods, err := m.k8sClient.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to list pods: %v", err)
		return
	}
	for _, p := range pods.Items {
		m.handlePodAdd(&p)
	}
}

func (m *Manager) Start() error {
	log.Println("Starting manager...")

	// Remove stale socket if it exists
	if err := os.Remove(ManagerSocket); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove stale socket: %v", err)
	}

	// Create socket directory
	if err := os.MkdirAll(filepath.Dir(ManagerSocket), 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %v", err)
	}

	// Start Unix socket listener
	l, err := net.Listen("unix", ManagerSocket)
	if err != nil {
		return fmt.Errorf("failed to create listener: %v", err)
	}
	m.listener = l

	// Ensure pinned map is available before watches
	m.waitForPinnedMap()

	// Load existing pods into the BPF map before starting watches
	m.syncPods()

	// Watch K8s events in goroutine
	go func() {
		for {
			m.watchPods()
			time.Sleep(5 * time.Second) // Retry on failure
		}
	}()

	// Handle CNI requests
	return m.serve()
}

func (m *Manager) Cleanup() error {
	log.Println("Cleaning up manager resources...")

	if m.listener != nil {
		m.listener.Close()
	}

	// Remove socket
	if err := os.Remove(ManagerSocket); err != nil && !os.IsNotExist(err) {
		log.Printf("Failed to remove socket: %v", err)
	}

	// Remove BPF map
	if m.depMap != nil && m.depMap.Path != "" {
		if err := os.Remove(m.depMap.Path); err != nil && !os.IsNotExist(err) {
			log.Printf("Failed to remove BPF map: %v", err)
			return err
		}
	}
	return nil
}
