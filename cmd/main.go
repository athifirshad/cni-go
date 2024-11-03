package main

import (
	"crypto/sha1"
	"encoding/hex"
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

// Update cmdAdd with better error handling
func cmdAdd(args *skel.CmdArgs) error {
	// Add debug logging
	fmt.Fprintf(os.Stderr, "Debug: Starting ADD for container %s\n", args.ContainerID)

	// Parse network configuration
	conf := &NetConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to parse network config: %v", err)
	}

	// Generate interface names
	hostVethName := generateVethName(args.ContainerID, args.IfName)
	fmt.Fprintf(os.Stderr, "Debug: args.ContainerID = %s\n", args.ContainerID)
	fmt.Fprintf(os.Stderr, "Debug: hostVethName = %s\n", hostVethName)
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

	// Add more verbose error handling for veth creation
	if err := netlink.LinkAdd(veth); err != nil {
		fmt.Fprintf(os.Stderr, "Debug: Failed to create veth pair: %v\n", err)
		// Try cleanup before returning error
		if link, err := netlink.LinkByName(hostVethName); err == nil {
			netlink.LinkDel(link)
		}
		return fmt.Errorf("failed to create veth pair: %v", err)
	}

	fmt.Fprintf(os.Stderr, "Debug: Successfully created veth pair %s <-> %s\n",
		hostVethName, contVethName)

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

// Update generateVethName to include namespace
func generateVethName(containerID string, ifName string) string {
	h := sha1.New()
	// Add random component to avoid collisions
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
