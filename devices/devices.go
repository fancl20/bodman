package devices

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

var (
	// ErrNotADevice denotes that a file is not a valid linux device.
	ErrNotADevice = errors.New("not a device node")
)

// Testing dependencies
var (
	unixLstat     = unix.Lstat
	ioutilReadDir = ioutil.ReadDir
)

// Given the path to a device and its cgroup_permissions(which cannot be easily queried) look up the
// information about a linux device and return that information as a Device struct.
func DeviceFromPath(path, permissions string) (*Device, error) {
	var stat unix.Stat_t
	err := unixLstat(path, &stat)
	if err != nil {
		return nil, err
	}

	var (
		devType   DeviceType
		mode      = stat.Mode
		devNumber = uint64(stat.Rdev)
		major     = unix.Major(devNumber)
		minor     = unix.Minor(devNumber)
	)
	switch {
	case mode&unix.S_IFBLK == unix.S_IFBLK:
		devType = BlockDevice
	case mode&unix.S_IFCHR == unix.S_IFCHR:
		devType = CharDevice
	case mode&unix.S_IFIFO == unix.S_IFIFO:
		devType = FifoDevice
	default:
		return nil, ErrNotADevice
	}
	return &Device{
		DeviceRule: DeviceRule{
			Type:        devType,
			Major:       int64(major),
			Minor:       int64(minor),
			Permissions: DevicePermissions(permissions),
		},
		Path:     path,
		FileMode: os.FileMode(mode),
		Uid:      stat.Uid,
		Gid:      stat.Gid,
	}, nil
}

// HostDevices returns all devices that can be found under /dev directory.
func HostDevices() ([]*Device, error) {
	return GetDevices("/dev")
}

// GetDevices recursively traverses a directory specified by path
// and returns all devices found there.
func GetDevices(path string) ([]*Device, error) {
	files, err := ioutilReadDir(path)
	if err != nil {
		return nil, err
	}
	var out []*Device
	for _, f := range files {
		switch {
		case f.IsDir():
			switch f.Name() {
			// ".lxc" & ".lxd-mounts" added to address https://github.com/lxc/lxd/issues/2825
			// ".udev" added to address https://github.com/opencontainers/runc/issues/2093
			case "pts", "shm", "fd", "mqueue", ".lxc", ".lxd-mounts", ".udev":
				continue
			default:
				sub, err := GetDevices(filepath.Join(path, f.Name()))
				if err != nil {
					return nil, err
				}

				out = append(out, sub...)
				continue
			}
		case f.Name() == "console":
			continue
		}
		device, err := DeviceFromPath(filepath.Join(path, f.Name()), "rwm")
		if err != nil {
			if err == ErrNotADevice {
				continue
			}
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		out = append(out, device)
	}
	return out, nil
}

func CreateDevicesFromHost(rootfs string) error {
	devices, err := GetDevices("/dev")
	if err != nil {
		return err
	}
	for _, node := range devices {
		if node.Major == 0 {
			continue
		}
		// containers running in a user namespace are not allowed to mknod
		// devices so we can just bind mount it from the host.
		// Note: currently we only support running uder root.
		if err := createDeviceNode(rootfs, node, false); err != nil {
			return err
		}
	}
	return nil
}

func bindMountDeviceNode(dest string, node *Device) error {
	f, err := os.Create(dest)
	if err != nil && !os.IsExist(err) {
		return err
	}
	if f != nil {
		f.Close()
	}
	return unix.Mount(node.Path, dest, "bind", unix.MS_BIND, "")
}

func createDeviceNode(rootfs string, node *Device, bind bool) error {
	if node.Path == "" {
		// The node only exists for cgroup reasons, ignore it here.
		return nil
	}
	dest := filepath.Join(rootfs, node.Path)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	if bind {
		return bindMountDeviceNode(dest, node)
	}
	if err := mknodDevice(dest, node); err != nil {
		if os.IsExist(err) {
			return nil
		} else if os.IsPermission(err) {
			return bindMountDeviceNode(dest, node)
		}
		return err
	}
	return nil
}

func mknodDevice(dest string, node *Device) error {
	fileMode := node.FileMode
	switch node.Type {
	case BlockDevice:
		fileMode |= unix.S_IFBLK
	case CharDevice:
		fileMode |= unix.S_IFCHR
	case FifoDevice:
		fileMode |= unix.S_IFIFO
	default:
		return fmt.Errorf("%c is not a valid device type for device %s", node.Type, node.Path)
	}
	dev, err := node.Mkdev()
	if err != nil {
		return err
	}
	if err := unix.Mknod(dest, uint32(fileMode), int(dev)); err != nil {
		return err
	}
	return unix.Chown(dest, int(node.Uid), int(node.Gid))
}
