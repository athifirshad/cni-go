// pkg/dependencies/map.go
package dependencies

import (
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"path/filepath"

	"github.com/cilium/ebpf"
)

type DependencyMap struct {
	Map  *ebpf.Map
	Path string
}

func (d *DependencyMap) Close() error {
	if d.Map != nil {
		return d.Map.Close()
	}
	return nil
}

// Update map spec to match container policy format
func NewDependencyMap() (*DependencyMap, error) {
	// Create BPF filesystem directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(BPFMapPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create BPF directory: %v", err)
	}

	// Try to load existing map first
	if m, err := ebpf.LoadPinnedMap(BPFMapPath, nil); err == nil {
		return &DependencyMap{Map: m, Path: BPFMapPath}, nil
	}

	// Create new map if loading failed
	spec := &ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    8, // uint64 for container hash
		ValueSize:  8, // Increased to store more policy flags
		MaxEntries: 10000,
	}

	m, err := ebpf.NewMap(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create map: %v", err)
	}

	if err := m.Pin(BPFMapPath); err != nil {
		m.Close()
		return nil, fmt.Errorf("failed to pin map: %v", err)
	}

	return &DependencyMap{Map: m, Path: BPFMapPath}, nil
}

// Add debug function
func (d *DependencyMap) DumpContents() {
	var (
		key   uint64
		value []byte
	)

	entries := d.Map.Iterate()
	for entries.Next(&key, &value) {
		restricted := value[0]&1 != 0
		log.Printf("Container Hash: %x, Restricted: %v", key, restricted)
	}
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
	return ebpf.LoadPinnedMap(path, &ebpf.LoadPinOptions{})
}

func (m *Manager) updateBPFMaps() error {
	log.Printf("Updating BPF maps with %d containers", len(m.networkMap.containers))
    
    bpfMap, err := ebpf.LoadPinnedMap(BPFMapPath, &ebpf.LoadPinOptions{})
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
        key uint64
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
