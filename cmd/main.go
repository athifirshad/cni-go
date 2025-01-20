package main

import (
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
)

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
	bpfMap, err := dependencies.LoadBPFMap(dependencies.BPFMapPath)
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
		// ...
	}

	conf := &NetConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	// Get container network details
	// containerInfo := &dependencies.ContainerNetwork{
	// 	ContainerID: args.ContainerID,
	// 	Interface:   args.IfName,
	// }

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

	r, err := ipam.ExecAdd(conf.IPAM.Type, args.StdinData)
	if err != nil {
		return fmt.Errorf("failed to run IPAM: %v", err)
	}

	result, err := current.NewResultFromResult(r)
	if err != nil {
		return fmt.Errorf("failed to parse IPAM result: %v", err)
	}

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
		Mac:     contVeth.Attrs().HardwareAddr.String(),
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
