package network

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containernetworking/cni/libcni"
	"github.com/urfave/cli/v2"
	"github.com/vishvananda/netns"
)

func getNetNS(containerID string) string {
	return fmt.Sprintf("cni-%s", containerID)
}

type Network struct {
	NetworkName      string
	RuntimeConfig    *libcni.RuntimeConf
	CNIConfigDir     string
	CNIPluginDir     []string
	NetworkNamespace string
}

func NewNetwork(ctx *cli.Context, hostname, containerID string) (*Network, error) {
	networkNamespace := fmt.Sprintf("cni-%s", containerID)
	cniArgs := [][2]string{
		{"IgnoreUnknown", "1"},
		{"K8S_POD_NAME", hostname},
		{"K8S_POD_NAMESPACE", networkNamespace},
		{"K8S_POD_INFRA_CONTAINER_ID", containerID},
	}
	capabilityArgs := make(map[string]interface{})

	portMappings, err := createPortBindings(ctx.StringSlice("publish"))
	if err != nil {
		return nil, err
	}
	if len(portMappings) > 0 {
		capabilityArgs["portMappings"] = portMappings
	}

	return &Network{
		NetworkName: ctx.String("network"),
		RuntimeConfig: &libcni.RuntimeConf{
			ContainerID:    containerID,
			NetNS:          filepath.Join("/var/run/netns", networkNamespace),
			IfName:         "eth0",
			Args:           cniArgs,
			CapabilityArgs: capabilityArgs,
		},
		CNIConfigDir:     ctx.String("cni-config-dir"),
		CNIPluginDir:     ctx.StringSlice("cni-plugin-dir"),
		NetworkNamespace: networkNamespace,
	}, nil
}

func Load(path string) (*Network, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	var n Network
	if err := json.NewDecoder(f).Decode(&n); err != nil {
		return nil, err
	}
	return &n, nil
}

func Dump(path string, n *Network) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(n); err != nil {
		return err
	}
	return nil
}

func (n *Network) Execute() error {
	switch n.NetworkName {
	case "host":
		return nil
	case "none":
		if _, err := netns.New(); err != nil {
			return err
		}
		return nil
	default:
		// Save current network namespace
		oldNS, err := netns.Get()
		if err != nil {
			return fmt.Errorf("Get old netns failed: %w", err)
		}
		defer oldNS.Close()
		// Create new network namespace
		newNS, err := netns.NewNamed(n.NetworkNamespace)
		if err != nil {
			return fmt.Errorf("Create new netns failed: %w", err)
		}
		defer newNS.Close()
		// Back to old network namspace to load CNI plugin
		if err := netns.Set(oldNS); err != nil {
			return fmt.Errorf("Back to old netns failed: %w", err)
		}
		// Add CNI Network
		netconf, err := libcni.LoadConfList(n.CNIConfigDir, n.NetworkName)
		if err != nil {
			return fmt.Errorf("Load cni config failed: %w", err)
		}

		cninet := libcni.NewCNIConfig(n.CNIPluginDir, nil)
		if _, err := cninet.AddNetworkList(context.TODO(), netconf, n.RuntimeConfig); err != nil {
			return fmt.Errorf("Add cni network failed: %w", err)
		}
		// New namespace is ready for Use
		return netns.Set(newNS)
	}

}

func (n *Network) Remove() error {
	switch n.NetworkName {
	case "host":
		return nil
	case "none":
		return nil
	default:
		netconf, err := libcni.LoadConfList(n.CNIConfigDir, n.NetworkName)
		if err != nil {
			return err
		}
		cninet := libcni.NewCNIConfig(n.CNIPluginDir, nil)
		if err := cninet.DelNetworkList(context.TODO(), netconf, n.RuntimeConfig); err != nil {
			return err
		}
		return netns.DeleteNamed(n.NetworkNamespace)
	}
}
