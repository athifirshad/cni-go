package dependencies

import (
	"sync"
)

const (
	BPFMapPath = "/sys/fs/bpf/container_deps"
)

type ContainerNetwork struct {
	ContainerID string            `json:"container_id"`
	PodName     string            `json:"pod_name"`
	Namespace   string            `json:"namespace"`
	IPAddress   string            `json:"ip_address"`
	MACAddress  string            `json:"mac_address"`
	Interface   string            `json:"interface"`
	Labels      map[string]string `json:"labels"`
}

type NetworkMap struct {
	containers map[string]*ContainerNetwork
	mutex      sync.RWMutex
}

func NewNetworkMap() *NetworkMap {
	return &NetworkMap{
		containers: make(map[string]*ContainerNetwork),
	}
}
