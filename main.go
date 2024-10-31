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
)

// PluginConf defines the configuration options for the plugin
type PluginConf struct {
	types.NetConf // Embeds standard CNI network configuration
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

// cmdAdd is called by the runtime when a container needs to be added to the network
func cmdAdd(args *skel.CmdArgs) error {
	// Parse network configuration from stdin
	conf, err := loadConf(args.StdinData)
	if err != nil {
		return fmt.Errorf("failed to load netconf: %w", err)
	}

	// For this demo, we don't do any actual networking.
	// Just log the information and return success.

	log.Printf("Demo CNI Plugin: ADD called with conf: %#v\n", conf)
	log.Printf("Container ID: %s, Netns: %s, IfName: %s\n", args.ContainerID, args.Netns, args.IfName)

	// Return a dummy result
	return printResult(conf, args.IfName)
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
