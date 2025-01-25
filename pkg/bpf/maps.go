package bpf

import (
    "fmt"
    "github.com/cilium/ebpf"
)

const (
    MapNameContainers = "containers"
    MapNamePolicies   = "policies"
)

type ContainerInfo struct {
    ContainerID string
    PodName     string
    Namespace   string
    NetNS       uint32
    IFName      string
    IP          [16]byte // IPv6-sized array to handle both v4/v6
}

var (
    ContainerMap *ebpf.Map
    PolicyMap   *ebpf.Map
)

func InitMaps() error {
    var err error
    
    // Container map: key = container ID, value = container info
    ContainerMap, err = ebpf.NewMap(&ebpf.MapSpec{
        Type:       ebpf.Hash,
        KeySize:    64,  // Container ID string length
        ValueSize:  256, // Size of ContainerInfo struct
        MaxEntries: 10000,
    })
    if err != nil {
        return fmt.Errorf("failed to create container map: %v", err)
    }

    // Policy map for network policies
    PolicyMap, err = ebpf.NewMap(&ebpf.MapSpec{
        Type:       ebpf.Hash,
        KeySize:    128, // Policy namespace/name
        ValueSize:  1024, // Policy data
        MaxEntries: 1000,
    })
    if err != nil {
        return fmt.Errorf("failed to create policy map: %v", err)
    }

    return nil
}

func PutContainer(id string, info ContainerInfo) error {
    return ContainerMap.Put([]byte(id), info)
}

func GetContainer(id string) (*ContainerInfo, error) {
    var info ContainerInfo
    err := ContainerMap.Lookup([]byte(id), &info)
    if err != nil {
        return nil, err
    }
    return &info, nil
}

func DeleteContainer(id string) error {
    return ContainerMap.Delete([]byte(id))
}
