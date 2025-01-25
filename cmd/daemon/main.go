package main

import (
	"encoding/json"
	"net"
	"os"
	"sync"
	"time"

	"github.com/athifirshad/go-cni/pkg/bpf"
	"github.com/athifirshad/go-cni/pkg/logging"
	"github.com/athifirshad/go-cni/pkg/store"
	"github.com/athifirshad/go-cni/pkg/types"
	"github.com/athifirshad/go-cni/pkg/version"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1" // <-- add missing import
)

var (
	logger       = logging.NewLogger("daemon")
	networkStore = store.DefaultStore
)

const (
	sockPath   = "/var/run/cni/daemon.sock" // Updated to match manager path
	STATE_FILE = "/var/run/cni/network-state.json"
)

// Add NetworkState type definition
type NetworkState struct {
	Pods       map[string]*v1.Pod
	Policies   map[string]*networkingv1.NetworkPolicy
	Containers map[string]*store.ContainerInfo
	mu         sync.RWMutex
}

func main() {
	logger.Info("Starting CNI daemon version %s", version.GetVersion())
	if err := bpf.InitMaps(); err != nil {
		logger.Error("Failed to initialize BPF maps: %v", err)
		os.Exit(1)
	}

	// Start background tasks
	go printBPFState()
	go printStoreState()
	go watchStateFile()

	// Setup socket
	if err := os.MkdirAll("/var/run/cni", 0755); err != nil {
		logger.Error("Failed to create socket directory: %v", err)
		os.Exit(1)
	}
	if err := os.Chmod("/var/run/cni", 0755); err != nil {
		logger.Error("Failed to set socket directory permissions: %v", err)
		os.Exit(1)
	}
	if err := os.RemoveAll(sockPath); err != nil {
		logger.Warn("Could not remove existing socket: %v", err)
	}
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		logger.Error("Failed to listen on socket: %v", err)
		os.Exit(1)
	}
	logger.Info("CNI daemon listening on %s", sockPath)
	logger.Info("Node ID: %s", os.Getenv("NODE_NAME"))

	// Accept connections in separate goroutine
	go func() {
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				logger.Warn("Failed to accept connection: %v", err)
				continue
			}
			go handleConnection(conn)
		}
	}()

	// Keep main alive
	select {}
}

func printStoreState() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		logger.Info("Current Network State:%s", networkStore.String())
	}
}

func printBPFState() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		logger.Info("Current BPF Map State:")

		// Print containers
		iter := bpf.ContainerMap.Iterate()
		var (
			key   []byte
			value bpf.ContainerInfo
		)
		for iter.Next(&key, &value) {
			logger.Info("Container: %s, Pod: %s/%s, NetNS: %d",
				string(key), value.Namespace, value.PodName, value.NetNS)
		}
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	remoteAddr := conn.RemoteAddr().String()
	logger.Debug("New connection from %s", remoteAddr)
	decoder := json.NewDecoder(conn)
	// Read the request first
	var rawMessage json.RawMessage
	if err := decoder.Decode(&rawMessage); err != nil {
		logger.Error("Failed to decode raw message: %v", err)
		return
	}
	// Decode command type first
	var commandType struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(rawMessage, &commandType); err != nil {
		logger.Error("Failed to decode command type: %v", err)
		return
	}
	var resp types.CNIResponse
	switch commandType.Command {
	case "ADD":
		var cniReq types.CNIRequest
		if err := json.Unmarshal(rawMessage, &cniReq); err != nil {
			logger.Error("Failed to decode CNI request: %v", err)
			resp = types.CNIResponse{Success: false, ErrorMsg: err.Error()}
		} else {
			// Add container to store
			networkStore.AddContainer(&store.ContainerInfo{
				ID:     cniReq.ContainerID,
				NetNS:  cniReq.Netns,
				IfName: cniReq.IfName,
			})
			resp = types.CNIResponse{Success: true}
		}
	case "DEL":
		var cniReq types.CNIRequest
		if err := json.Unmarshal(rawMessage, &cniReq); err != nil {
			logger.Error("Failed to decode CNI request: %v", err)
			resp = types.CNIResponse{Success: false, ErrorMsg: err.Error()}
		} else {
			// Remove container from store
			networkStore.DeleteContainer(cniReq.ContainerID)
			resp = types.CNIResponse{Success: true}
		}
	case "POD_EVENT":
		var podEvent types.PodEvent
		if err := json.Unmarshal(rawMessage, &podEvent); err != nil {
			logger.Error("Failed to decode pod event: %v", err)
			resp = types.CNIResponse{Success: false, ErrorMsg: err.Error()}
		} else {
			if podEvent.Event == "POD_ADDED" {
				networkStore.AddPod(podEvent.Pod)
			} else if podEvent.Event == "POD_DELETED" {
				networkStore.DeletePod(podEvent.Pod.Namespace, podEvent.Pod.Name)
			}
			resp = types.CNIResponse{Success: true}
		}

	case "RECONCILE":
		var reconcileReq types.ReconcileRequest
		if err := json.Unmarshal(rawMessage, &reconcileReq); err != nil {
			logger.Error("Failed to decode reconcile request: %v", err)
			resp = types.CNIResponse{Success: false, ErrorMsg: err.Error()}
		} else {
			networkStore.Reconcile(reconcileReq.Pods, reconcileReq.Policies)
			resp = types.CNIResponse{Success: true}
		}

	case "CHECK":
		var cniReq types.CNIRequest
		if err := json.Unmarshal(rawMessage, &cniReq); err != nil {
			logger.Error("Failed to decode CNI request: %v", err)
			resp = types.CNIResponse{Success: false, ErrorMsg: err.Error()}
		} else {
			logger.Info("Processing CNI request: %+v", cniReq)
			resp = types.CNIResponse{Success: true}
		}

	default:
		resp = types.CNIResponse{Success: false, ErrorMsg: "unknown command"}
	}

	// Send response with error handling
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		logger.Error("Failed to send response: %v", err)
	}
}

func watchStateFile() {
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		data, err := os.ReadFile(STATE_FILE) // Changed from ioutil.ReadFile
		if err != nil {
			logger.Error("Failed to read state: %v", err)
			continue
		}

		var state NetworkState
		if err := json.Unmarshal(data, &state); err != nil {
			logger.Error("Failed to parse state: %v", err)
			continue
		}

		logger.Info("Current Network State:")
		state.mu.RLock()
		for k, pod := range state.Pods {
			logger.Info("Pod: %s Phase: %s", k, pod.Status.Phase)
		}
		state.mu.RUnlock()
	}
}
