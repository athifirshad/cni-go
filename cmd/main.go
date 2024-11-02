package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ipam"
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
	// Parse network configuration
	conf := &NetConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to parse network config: %v", err)
	}

	// Generate interface names
	hostVethName := generateVethName(args.ContainerID)
	contVethName := args.IfName

	// Check if veth already exists and clean up if needed
	if link, err := netlink.LinkByName(hostVethName); err == nil {
		if err := netlink.LinkDel(link); err != nil {
			return fmt.Errorf("failed to delete existing veth %q: %v", hostVethName, err)
		}
	}

	// Create veth pair
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: hostVethName,
			MTU:  conf.MTU,
		},
		PeerName: contVethName,
	}

	if err := netlink.LinkAdd(veth); err != nil {
		return fmt.Errorf("failed to create veth pair: %v", err)
	}

	// Get host veth interface
	hostVeth, err := netlink.LinkByName(hostVethName)
	if err != nil {
		return fmt.Errorf("failed to lookup host veth %q: %v", hostVethName, err)
	}

	// Bring host veth up
	if err := netlink.LinkSetUp(hostVeth); err != nil {
		return fmt.Errorf("failed to set %q up: %v", hostVethName, err)
	}

	// Call IPAM plugin
	r, err := ipam.ExecAdd(conf.IPAM.Type, args.StdinData)
	if err != nil {
		return fmt.Errorf("IPAM add failed: %v", err)
	}

	// Convert result to current version
	result, err := current.NewResultFromResult(r)
	if err != nil {
		return fmt.Errorf("failed to convert IPAM result: %v", err)
	}

	return types.PrintResult(result, conf.CNIVersion)
}

// Helper functions
func validateNetConf(conf *NetConf) error {
	if conf.MTU == 0 {
		conf.MTU = 1500
	}

	if conf.IPAM.Type == "" {
		return fmt.Errorf("IPAM config missing")
	}
	return nil
}

func generateVethName(containerID string) string {
	// Return a name like "caliXXXXXXX" where XXXXXXX is derived from container ID
	return fmt.Sprintf("cali%s", containerID[:min(11, len(containerID))])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

// loadConf loads the network configuration from the provided data
func loadConf(bytes []byte) (*PluginConf, error) {
	n := &PluginConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, fmt.Errorf("failed to parse network configuration: %w", err)
	}
	return n, nil
}

// printResult prints a dummy result for this demo plugin
func printResult(conf *PluginConf, ifName string) error {
	result := &current.Result{
		CNIVersion: current.ImplementedSpecVersion,
		Interfaces: []*current.Interface{
			{
				Name: ifName,
				// Add more interface details if needed
			},
		},
		// Add IP addresses, routes, DNS, etc. as needed
	}

	return types.PrintResult(result, conf.CNIVersion)
}
