package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"net"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

// PluginConf defines the configuration options for the plugin
type PluginConf struct {
	types.NetConf // Embeds standard CNI network configuration
}

// NetConf defines the network configuration for the plugin
type NetConf struct {
	types.NetConf
	MTU int `json:"mtu"`
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

func cmdAdd(args *skel.CmdArgs) error {
	fmt.Fprintf(os.Stderr, "Debug: Starting ADD for container %s\n", args.ContainerID)

	// Parse network configuration
	conf := &NetConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	// Get container network namespace
	containerNS, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to get container netns: %v", err)
	}
	defer containerNS.Close()

	// Create interfaces within container namespace
	var hostInterface, containerInterface *current.Interface
	err = containerNS.Do(func(netNS ns.NetNS) error {
		// Setup veth pair inside container namespace
		hostVeth, containerVeth, err := ip.SetupVeth(args.IfName, conf.MTU, generateVethName(args.ContainerID, args.IfName), netNS)
		if err != nil {
			return fmt.Errorf("failed to setup veth: %v", err)
		}

		hostInterface = &current.Interface{
			Name: hostVeth.Name,
			Mac:  hostVeth.HardwareAddr.String(),
		}
		containerInterface = &current.Interface{
			Name:    containerVeth.Name,
			Mac:     containerVeth.HardwareAddr.String(),
			Sandbox: netNS.Path(),
		}
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Debug: Created veth pair %s <-> %s\n",
		hostInterface.Name, containerInterface.Name)

	// Call IPAM plugin
	r, err := ipam.ExecAdd(conf.IPAM.Type, args.StdinData)
	if err != nil {
		return fmt.Errorf("IPAM failed: %v", err)
	}

	// Convert to current version
	result, err := current.NewResultFromResult(r)
	if err != nil {
		return fmt.Errorf("failed to convert result: %v", err)
	}

	// Configure IP address in container namespace
	err = containerNS.Do(func(netNS ns.NetNS) error {
		// Get container interface
		containerVeth, err := netlink.LinkByName(containerInterface.Name)
		if err != nil {
			return fmt.Errorf("failed to lookup %q: %v", containerInterface.Name, err)
		}

		// Add IP addresses to container interface
		for _, ipc := range result.IPs {
			addr := &netlink.Addr{
				IPNet: &net.IPNet{
					IP:   ipc.Address.IP,
					Mask: ipc.Address.Mask,
				},
			}
			if err := netlink.AddrAdd(containerVeth, addr); err != nil {
				return fmt.Errorf("failed to add IP addr %v to %q: %v", addr, containerInterface.Name, err)
			}
			fmt.Fprintf(os.Stderr, "Debug: Added IP %v to %s\n", addr, containerInterface.Name)
		}

		// Set container interface up
		if err := netlink.LinkSetUp(containerVeth); err != nil {
			return fmt.Errorf("failed to set %q up: %v", containerInterface.Name, err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	return types.PrintResult(result, conf.CNIVersion)
}

// Update generateVethName to include namespace
func generateVethName(containerID string, ifName string) string {
	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%s-%s-%d", containerID, ifName, os.Getpid())))
	sha := hex.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("veth%s", sha[:11])
}

// cmdCheck is called by the runtime to check the status of a container's network
func cmdCheck(args *skel.CmdArgs) error {
	// For this demo, always return success (no actual checks)
	log.Println("Demo CNI Plugin: CHECK called (always succeeding)")
	return nil
}

// cmdDel is called by the runtime when a container is removed from the network
func cmdDel(args *skel.CmdArgs) error {
	// For this demo, just log and return success
	log.Println("Demo CNI Plugin: DEL called (no-op)")
	return nil
}
