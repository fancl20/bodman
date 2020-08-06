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

func NewNetwork(ctx *cli.Context, containerID string) *Network {
	networkNamespace := fmt.Sprintf("cni-%s", containerID)
	cniArgs := [][2]string{
		{"IgnoreUnknown", "1"},
		{"K8S_POD_NAME", containerID},
	}
	return &Network{
		NetworkName: ctx.String("network"),
		RuntimeConfig: &libcni.RuntimeConf{
			ContainerID: containerID,
			NetNS:       filepath.Join("/var/run/netns", networkNamespace),
			IfName:      "eth0",
			Args:        cniArgs,
		},
		CNIConfigDir:     ctx.String("cni-config-dir"),
		CNIPluginDir:     ctx.StringSlice("cni-plugin-dir"),
		NetworkNamespace: networkNamespace,
	}
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
			return err
		}
		// Create new network namespace
		newNS, err := netns.NewNamed(n.NetworkNamespace)
		if err != nil {
			return err
		}
		// Back to old network namspace to load CNI plugin
		if err := netns.Set(oldNS); err != nil {
			return err
		}
		// Add CNI Network
		netconf, err := libcni.LoadConfList(n.CNIConfigDir, n.NetworkName)
		if err != nil {
			return err
		}

		cninet := libcni.NewCNIConfig(n.CNIPluginDir, nil)
		if _, err := cninet.AddNetworkList(context.TODO(), netconf, n.RuntimeConfig); err != nil {
			return err
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
