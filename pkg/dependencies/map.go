// pkg/dependencies/map.go
package dependencies

import (
	"fmt"
	"hash/fnv"
	"log"
	"os"

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

// Update container format to use 8 bytes
func containerToBPFFormat(c *ContainerNetwork) []byte {
	buf := make([]byte, 8) // Increased buffer size
	if c.Labels["network.policy"] == "restricted" {
		buf[0] |= 1 // First byte stores policy flags
		log.Printf("Container %s marked as restricted (labels: %v)",
			c.ContainerID, c.Labels)
	} else {
		log.Printf("Container %s not restricted (labels: %v)",
			c.ContainerID, c.Labels)
	}
	log.Printf("Setting container %s policy: restricted=%v",
		c.ContainerID[:12], buf[0]&1 != 0)
	return buf
}

func LoadBPFMap(path string) (*ebpf.Map, error) {
	log.Printf("Loading BPF map from: %s", path)

	// Attempt to fix permissions before loading
	if err := os.Chmod(path, 0644); err != nil {
		log.Printf("Warning: Could not set map permissions: %v", err)
	}

	// Check if file exists and is accessible
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("cannot access map file: %v", err)
	}

	m, err := ebpf.LoadPinnedMap(path, &ebpf.LoadPinOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to load map: %v (path: %s)", err, path)
	}

	log.Printf("Successfully loaded BPF map with FD: %d", m.FD())
	return m, nil
}

func (m *Manager) updateBPFMaps() error {
	// Get BPF map
	mapPath := "/sys/fs/bpf/container_map"
	bpfMap, err := ebpf.LoadPinnedMap(mapPath, &ebpf.LoadPinOptions{})
	if err != nil {
		return fmt.Errorf("failed to load BPF map: %v", err)
	}
	defer bpfMap.Close()

	m.networkMap.mutex.RLock()
	defer m.networkMap.mutex.RUnlock()

	for _, container := range m.networkMap.containers {
		key := Hash(container.ContainerID)
		value := containerToBPFFormat(container)

		if err := bpfMap.Update(&key, &value, ebpf.UpdateAny); err != nil {
			return fmt.Errorf("failed to update map entry for container %s: %v",
				container.ContainerID, err)
		}
		log.Printf("Added container %s to BPF map with hash %x",
			container.ContainerID, key)
	}

	// Verify contents
	var (
		key   uint64
		value []byte
	)
	entries := bpfMap.Iterate()
	count := 0
	for entries.Next(&key, &value) {
		count++
		log.Printf("BPF map entry - Hash: %x, Restricted: %v", key, value[0]&1 != 0)
	}
	log.Printf("Total entries in BPF map: %d", count)

	return nil
}
