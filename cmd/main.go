package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"syscall"

	"github.com/athifirshad/go-cni/pkg/dependencies"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

const (
	bridgeName = "cni0"
	mtu        = 1500
	mapPath    = "/sys/fs/bpf/container_map" // Path to the eBPF map
)

// ContainerInfo represents the data to store in the map.
const ifaceNameLen = 16 // Define a fixed length for interface names

// ContainerInfo represents the data to store in the map.
type ContainerInfo struct {
	IP        [4]byte  `binary:"fixed"`
	MAC       [6]byte  `binary:"fixed"`
	Interface [16]byte `binary:"fixed"`
}

func init() {
	runtime.LockOSThread()
}

type NetConf struct {
	types.NetConf
	MTU int `json:"mtu"`
}

func setupBridge() (*netlink.Bridge, error) {
	br := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name:   bridgeName,
			MTU:    mtu,
			TxQLen: -1,
		},
	}

	err := netlink.LinkAdd(br)
	if err != nil && err != syscall.EEXIST {
		return nil, fmt.Errorf("failed to create bridge: %v", err)
	}

	l, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup bridge: %v", err)
	}

	br, ok := l.(*netlink.Bridge)
	if !ok {
		return nil, fmt.Errorf("%s already exists but is not a bridge", bridgeName)
	}

	if err := netlink.LinkSetUp(br); err != nil {
		return nil, fmt.Errorf("failed to set bridge up: %v", err)
	}

	return br, nil
}

func lookupContainerPolicy(containerID string) (bool, error) {
	// Load BPF map
	bpfMap, err := dependencies.LoadBPFMap("/sys/fs/bpf/container_map")
	if err != nil {
		return false, fmt.Errorf("failed to load BPF map: %v", err)
	}
	defer bpfMap.Close()

	var value []byte
	key := dependencies.Hash(containerID)
	if err := bpfMap.Lookup(key, &value); err != nil {
		return false, nil // Default to unrestricted
	}

	return value[0]&1 != 0, nil // Check restricted flag
}

// AddContainerToMap adds the container's IP, MAC, and interface to the eBPF map.
func AddContainerToMap(ip net.IP, mac net.HardwareAddr, iface string) error {
	log.Printf("IP address passed: %s", ip.String())
	bpfMap, err := dependencies.LoadBPFMap(mapPath)
	if err != nil {
		return fmt.Errorf("failed to load BPF map: %v", err)
	}
	defer bpfMap.Close()

	// Key is derived from container IP (assuming IPv4).
	var key [4]byte
	copy(key[:], ip.To4())

	// Value contains the MAC and interface name.
	value := ContainerInfo{}
	fmt.Println("Size of ContainerInfo:", binary.Size(value))
	copy(value.IP[:], ip.To4())
	copy(value.MAC[:], mac)

	// Ensure the interface name fits within the fixed-size array
	if len(iface) > ifaceNameLen {
		return fmt.Errorf("interface name %s is too long", iface)
	}
	copy(value.Interface[:], iface)
	// Serialize the struct into binary format
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, value); err != nil {
		return fmt.Errorf("failed to serialize ContainerInfo: %v", err)
	}

	// Write to the BPF map
	if err := bpfMap.Update(key[:], buf.Bytes(), 0); err != nil {
		return fmt.Errorf("failed to update BPF map: %v", err)
	}

	log.Printf("Added to map: IP=%s, MAC=%s, Interface=%s\n", ip.String(), mac.String(), iface)
	return nil
}

func cmdAdd(args *skel.CmdArgs) error {
	log.Printf("CNI ADD called for container: %s", args.ContainerID)

	depMap, err := dependencies.NewDependencyMap()
	if err != nil {
		return fmt.Errorf("failed to create BPF map: %v", err)
	}
	defer depMap.Close()

	// Verify BPF map exists and is accessible
	if depMap.Map == nil {
		return fmt.Errorf("BPF map not initialized")
	}

	// Add BPF map lookup before network setup
	restricted, err := lookupContainerPolicy(args.ContainerID)
	if err != nil {
		log.Printf("Policy lookup error for %s: %v", args.ContainerID, err)
	} else {
		log.Printf("Container %s policy lookup result: restricted=%v",
			args.ContainerID, restricted)
	}

	if restricted {
		// Apply network restrictions
		// Additional restrictions can be implemented here
	}

	conf := &NetConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	br, err := setupBridge()
	if err != nil {
		return err
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close()

	var hostInterface, containerInterface net.Interface
	err = netns.Do(func(hostNS ns.NetNS) error {
		var err error
		hostInterface, containerInterface, err = ip.SetupVeth(args.IfName, mtu, "", hostNS)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Look up host interface in host namespace
	hostVeth, err := netlink.LinkByName(hostInterface.Name)
	if err != nil {
		return fmt.Errorf("failed to lookup host interface: %v", err)
	}

	log.Printf("Host Interface Name: %s", hostVeth)

	// Look up container interface in container namespace
	var contVeth netlink.Link
	err = netns.Do(func(_ ns.NetNS) error {
		var err error
		contVeth, err = netlink.LinkByName(containerInterface.Name)
		if err != nil {
			return fmt.Errorf("failed to lookup container interface: %v", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if err := netlink.LinkSetMaster(hostVeth, br); err != nil {
		return fmt.Errorf("failed to connect %q to bridge: %v", hostVeth.Attrs().Name, err)
	}

	log.Printf("Executing IPAM ExecAdd with config: %+v", args.StdinData)

	r, err := ipam.ExecAdd(conf.IPAM.Type, args.StdinData)
	if err != nil {
		log.Printf("Failed to run IPAM ExecAdd: %v", err)
		return fmt.Errorf("failed to run IPAM: %v", err)
	}

	log.Printf("IPAM Result raw: %+v", r)

	result, err := current.NewResultFromResult(r)
	if err != nil {
		return fmt.Errorf("failed to parse IPAM result: %v", err)
	}
	log.Printf("IPAM Result after parsing: %+v", result)

	// Extract container IP

	err = netns.Do(func(hostNS ns.NetNS) error {
		if err := netlink.LinkSetUp(contVeth); err != nil {
			return fmt.Errorf("failed to set %q up: %v", contVeth.Attrs().Name, err)
		}

		// Add IP to container interface
		for _, ipc := range result.IPs {
			addr := &netlink.Addr{IPNet: &ipc.Address}
			if err := netlink.AddrAdd(contVeth, addr); err != nil {
				return fmt.Errorf("failed to add IP addr to %q: %v", contVeth.Attrs().Name, err)
			}
		}

		containerIP := net.IP{}
		log.Printf("Container Interface Name1: %s", containerInterface.Name)
		log.Printf("Host Interface Name: %s", hostInterface.Name)

		err = netns.Do(func(_ ns.NetNS) error {
			link, err := netlink.LinkByName(containerInterface.Name)
			if err != nil {
				return err
			}

			addrList, err := netlink.AddrList(link, syscall.AF_INET)
			if err != nil {
				return err
			}
			if len(addrList) > 0 {
				containerIP = addrList[0].IP
			}
			log.Printf("AddrList: %+v", addrList)
			log.Printf("Container IP: %+s", containerIP)
			log.Printf("Host Interface Name: %s", hostInterface.Name)
			return nil
		})
		if err != nil {
			return err
		}

		// Add container details to eBPF map
		if err := AddContainerToMap(containerIP, containerInterface.HardwareAddr, args.IfName); err != nil {
			return err
		}

		// Add default route
		gw := result.IPs[0].Gateway
		if gw != nil {
			route := &netlink.Route{
				LinkIndex: contVeth.Attrs().Index,
				Gw:        gw,
				Dst:       nil,
			}
			if err := netlink.RouteAdd(route); err != nil {
				return fmt.Errorf("failed to add default route: %v", err)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	result.Interfaces = []*current.Interface{{
		Name:    args.IfName,
		Mac:     containerInterface.HardwareAddr.String(),
		Sandbox: netns.Path(),
	}}

	return types.PrintResult(result, conf.CNIVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	conf := &NetConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	if err := ipam.ExecDel(conf.IPAM.Type, args.StdinData); err != nil {
		return err
	}

	if args.Netns == "" {
		return nil
	}

	// Remove the interface
	err := ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
		if err := ip.DelLinkByName(args.IfName); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Remove the veth pair from the host
	hostVethName := fmt.Sprintf("veth%s", args.ContainerID[:5])
	hostVeth, err := netlink.LinkByName(hostVethName)
	if err == nil {
		if err := netlink.LinkDel(hostVeth); err != nil {
			return fmt.Errorf("failed to delete host veth %q: %v", hostVethName, err)
		}
	}

	return nil
}

func cmdCheck(args *skel.CmdArgs) error {
	return nil
}

func init() {
	// Setup version info
	version.PluginSupports("0.1.0", "0.2.0", "0.3.0", "0.4.0", "1.0.0")
}

func main() {
	// Setup logging
	log.SetOutput(os.Stderr)

	// Start the plugin
	skel.PluginMainFuncs(
		skel.CNIFuncs{
			Add:   cmdAdd,
			Check: cmdCheck,
			Del:   cmdDel,
		},
		version.All,
		"Demo CNI plugin",
	)

}
