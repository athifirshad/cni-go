// pkg/dependencies/map.go
package dependencies

import (
	"fmt"
	"hash/fnv"

	"github.com/cilium/ebpf"
)

type DependencyMap struct {
	FD   int    // eBPF map file descriptor
	Path string // Pin path for the map
}

func NewDependencyMap() (*DependencyMap, error) {
	// Create eBPF map specification
	spec := &ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    8, // Container ID hash
		ValueSize:  4, // Dependency flags
		MaxEntries: 10000,
	}

	// Create new eBPF map
	m, err := ebpf.NewMap(spec)
	if err != nil {
		return nil, err
	}

	// Pin the map to filesystem
	mapPath := "/sys/fs/bpf/container_deps"
	if err := m.Pin(mapPath); err != nil {
		return nil, err
	}

	return &DependencyMap{
		FD:   m.FD(),
		Path: mapPath,
	}, nil
}

// Hash generates a uint64 hash of the container ID
func Hash(containerID string) uint64 {
	h := fnv.New64()
	h.Write([]byte(containerID))
	return h.Sum64()
}

func containerToBPFFormat(c *ContainerNetwork) []byte {
	// Pack container info into BPF map format
	buf := make([]byte, 4) // 4 bytes for flags
	// Set appropriate flags based on container labels
	if c.Labels["network.policy"] == "restricted" {
		buf[0] |= 1 // Set restricted flag
	}
	return buf
}

func LoadBPFMap(path string) (*ebpf.Map, error) {
	return ebpf.LoadPinnedMap(path, &ebpf.LoadPinOptions{})
}

func (m *Manager) updateBPFMaps() error {
	// Get BPF map
	mapPath := "/sys/fs/bpf/container_map"
	bpfMap, err := ebpf.LoadPinnedMap(mapPath, &ebpf.LoadPinOptions{})
	if err != nil {
		return fmt.Errorf("failed to load BPF map: %v", err)
	}
	defer bpfMap.Close()

	// Update entries
	m.networkMap.mutex.RLock()
	defer m.networkMap.mutex.RUnlock()

	for _, container := range m.networkMap.containers {
		key := Hash(container.ContainerID)
		value := containerToBPFFormat(container)

		if err := bpfMap.Update(key, value, ebpf.UpdateAny); err != nil {
			return fmt.Errorf("failed to update map entry: %v", err)
		}
	}

	return nil
}
