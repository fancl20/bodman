package network

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	protoTCP = "tcp"
	protoUDP = "udp"
)

// PortMapping maps to the standard CNI portmapping Capability
// see: https://github.com/containernetworking/cni/blob/master/CONVENTIONS.md
type PortMapping struct {
	// HostPort is the port number on the host.
	HostPort int32 `json:"hostPort"`
	// ContainerPort is the port number inside the sandbox.
	ContainerPort int32 `json:"containerPort"`
	// Protocol is the protocol of the port mapping.
	Protocol string `json:"protocol"`
	// HostIP is the host ip to use.
	HostIP string `json:"hostIP"`
}

// createPortBindings iterates ports mappings into SpecGen format.
func createPortBindings(ports []string) ([]PortMapping, error) {
	// --publish is formatted as follows:
	// [[hostip:]hostport[-endPort]:]containerport[-endPort][/protocol]
	toReturn := make([]PortMapping, 0, len(ports))

	for _, p := range ports {
		var (
			ctrPort                 string
			proto, hostIP, hostPort *string
		)

		splitProto := strings.Split(p, "/")
		switch len(splitProto) {
		case 1:
			// No protocol was provided
		case 2:
			proto = &(splitProto[1])
		default:
			return nil, fmt.Errorf("invalid port format - protocol can only be specified once")
		}

		remainder := splitProto[0]
		haveV6 := false

		// Check for an IPv6 address in brackets
		splitV6 := strings.Split(remainder, "]")
		switch len(splitV6) {
		case 1:
			// Do nothing, proceed as before
		case 2:
			// We potentially have an IPv6 address
			haveV6 = true
			if !strings.HasPrefix(splitV6[0], "[") {
				return nil, fmt.Errorf("invalid port format - IPv6 addresses must be enclosed by []")
			}
			if !strings.HasPrefix(splitV6[1], ":") {
				return nil, fmt.Errorf("invalid port format - IPv6 address must be followed by a colon (':')")
			}
			ipNoPrefix := strings.TrimPrefix(splitV6[0], "[")
			hostIP = &ipNoPrefix
			remainder = strings.TrimPrefix(splitV6[1], ":")
		default:
			return nil, fmt.Errorf("invalid port format - at most one IPv6 address can be specified in a --publish")
		}

		splitPort := strings.Split(remainder, ":")
		switch len(splitPort) {
		case 1:
			if haveV6 {
				return nil, fmt.Errorf("invalid port format - must provide host and destination port if specifying an IP")
			}
			ctrPort = splitPort[0]
		case 2:
			hostPort = &(splitPort[0])
			ctrPort = splitPort[1]
		case 3:
			if haveV6 {
				return nil, fmt.Errorf("invalid port format - when v6 address specified, must be [ipv6]:hostPort:ctrPort")
			}
			hostIP = &(splitPort[0])
			hostPort = &(splitPort[1])
			ctrPort = splitPort[2]
		default:
			return nil, fmt.Errorf("invalid port format - format is [[hostIP:]hostPort:]containerPort")
		}

		newPorts, err := parseSplitPort(hostIP, hostPort, ctrPort, proto)
		if err != nil {
			return nil, err
		}

		toReturn = append(toReturn, newPorts...)
	}

	return toReturn, nil
}

// parseSplitPort parses individual components of the --publish flag to produce
// a single port mapping in SpecGen format.
func parseSplitPort(hostIP, hostPort *string, ctrPort string, protocol *string) ([]PortMapping, error) {
	basePort := PortMapping{}
	if ctrPort == "" {
		return nil, fmt.Errorf("must provide a non-empty container port to publish")
	}
	ctrStart, ctrLen, err := parseAndValidateRange(ctrPort)
	if err != nil {
		return nil, fmt.Errorf("error parsing container port: %w", err)
	}
	basePort.ContainerPort = ctrStart

	if protocol != nil {
		basePort.Protocol = *protocol
		if basePort.Protocol == "" {
			basePort.Protocol = "tcp"
		}
	}

	if hostIP != nil {
		if *hostIP == "" {
			return nil, fmt.Errorf("must provide a non-empty container host IP to publish")
		} else if *hostIP != "0.0.0.0" {
			// If hostIP is 0.0.0.0, leave it unset - CNI treats
			// 0.0.0.0 and empty differently, Docker does not.
			testIP := net.ParseIP(*hostIP)
			if testIP == nil {
				return nil, fmt.Errorf("cannot parse %q as an IP address", *hostIP)
			}
			basePort.HostIP = testIP.String()
		}
	}

	if hostPort != nil {
		if *hostPort == "" {
			// Set 0 as a placeholder. The server side of Specgen
			// will find a random, open, unused port to use.
			basePort.HostPort = 0
		} else {
			hostStart, hostLen, err := parseAndValidateRange(*hostPort)
			if err != nil {
				return nil, fmt.Errorf("error parsing host port: %w", err)
			}
			if hostLen != ctrLen {
				return nil, fmt.Errorf("host and container port ranges have different lengths: %d vs %d", hostLen, ctrLen)
			}
			basePort.HostPort = hostStart
		}
	} else {
		basePort.HostPort = basePort.ContainerPort
	}

	var newPorts []PortMapping
	for i := int32(0); i < ctrLen; i++ {
		newPort := basePort
		newPort.ContainerPort += i
		if newPort.HostPort != 0 {
			newPort.HostPort += i
		}
		newPorts = append(newPorts, newPort)
	}
	return newPorts, nil
}

// Parse and validate a port range.
// Returns start port, length of range, error.
func parseAndValidateRange(portRange string) (int32, int32, error) {
	splitRange := strings.Split(portRange, "-")
	if len(splitRange) > 2 {
		return 0, 0, fmt.Errorf("invalid port format - port ranges are formatted as startPort-stopPort")
	}

	if splitRange[0] == "" {
		return 0, 0, fmt.Errorf("port numbers cannot be negative")
	}

	startPort, err := parseAndValidatePort(splitRange[0])
	if err != nil {
		return 0, 0, err
	}

	var rangeLen int32 = 1
	if len(splitRange) == 2 {
		if splitRange[1] == "" {
			return 0, 0, fmt.Errorf("must provide ending number for port range")
		}
		endPort, err := parseAndValidatePort(splitRange[1])
		if err != nil {
			return 0, 0, err
		}
		if endPort <= startPort {
			return 0, 0, fmt.Errorf("the end port of a range must be higher than the start port - %d is not higher than %d", endPort, startPort)
		}
		// Our range is the total number of ports
		// involved, so we need to add 1 (8080:8081 is
		// 2 ports, for example, not 1)
		rangeLen = endPort - startPort + 1
	}

	return startPort, rangeLen, nil
}

// Turn a single string into a valid U16 port.
func parseAndValidatePort(port string) (int32, error) {
	num, err := strconv.Atoi(port)
	if err != nil {
		return 0, fmt.Errorf("cannot parse %q as a port number: %w", port, err)
	}
	if num < 1 || num > 65535 {
		return 0, fmt.Errorf("port numbers must be between 1 and 65535 (inclusive), got %d", num)
	}
	return int32(num), nil
}
