package mount

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

var (
	ErrInvalidVolumn = errors.New("Invalid volumn format")
)

type Mount struct {
	// Source path for the mount.
	Source string `json:"source"`

	// Destination path for the mount inside the container.
	Destination string `json:"destination"`

	// Device the mount is for.
	Device string `json:"device"`

	// Mount flags.
	Flags uintptr `json:"flags"`

	// Propagation Flags
	PropagationFlags []uintptr `json:"propagation_flags"`

	// Mount data applied to the mount.
	Data string `json:"data"`

	// Relabel source if set, "z" indicates shared, "Z" indicates unshared.
	Relabel string `json:"relabel"`
}

// parseMountOptions parses the string and returns the flags, propagation
// flags and any mount data that it contains.
func parseMountOptions(options []string) (uintptr, []uintptr, string) {
	var (
		flag   uintptr
		pgflag []uintptr
		data   []string
	)
	flags := map[string]struct {
		clear bool
		flag  uintptr
	}{
		"acl":           {false, unix.MS_POSIXACL},
		"async":         {true, unix.MS_SYNCHRONOUS},
		"atime":         {true, unix.MS_NOATIME},
		"bind":          {false, unix.MS_BIND},
		"defaults":      {false, 0},
		"dev":           {true, unix.MS_NODEV},
		"diratime":      {true, unix.MS_NODIRATIME},
		"dirsync":       {false, unix.MS_DIRSYNC},
		"exec":          {true, unix.MS_NOEXEC},
		"iversion":      {false, unix.MS_I_VERSION},
		"loud":          {true, unix.MS_SILENT},
		"mand":          {false, unix.MS_MANDLOCK},
		"noacl":         {true, unix.MS_POSIXACL},
		"noatime":       {false, unix.MS_NOATIME},
		"nodev":         {false, unix.MS_NODEV},
		"nodiratime":    {false, unix.MS_NODIRATIME},
		"noexec":        {false, unix.MS_NOEXEC},
		"noiversion":    {true, unix.MS_I_VERSION},
		"nomand":        {true, unix.MS_MANDLOCK},
		"norelatime":    {true, unix.MS_RELATIME},
		"nostrictatime": {true, unix.MS_STRICTATIME},
		"nosuid":        {false, unix.MS_NOSUID},
		"rbind":         {false, unix.MS_BIND | unix.MS_REC},
		"relatime":      {false, unix.MS_RELATIME},
		"remount":       {false, unix.MS_REMOUNT},
		"ro":            {false, unix.MS_RDONLY},
		"rw":            {true, unix.MS_RDONLY},
		"silent":        {false, unix.MS_SILENT},
		"strictatime":   {false, unix.MS_STRICTATIME},
		"suid":          {true, unix.MS_NOSUID},
		"sync":          {false, unix.MS_SYNCHRONOUS},
	}
	propagationFlags := map[string]uintptr{
		"private":     unix.MS_PRIVATE,
		"shared":      unix.MS_SHARED,
		"slave":       unix.MS_SLAVE,
		"unbindable":  unix.MS_UNBINDABLE,
		"rprivate":    unix.MS_PRIVATE | unix.MS_REC,
		"rshared":     unix.MS_SHARED | unix.MS_REC,
		"rslave":      unix.MS_SLAVE | unix.MS_REC,
		"runbindable": unix.MS_UNBINDABLE | unix.MS_REC,
	}
	for _, o := range options {
		// If the option does not exist in the flags table or the flag
		// is not supported on the platform,
		// then it is a data value for a specific fs type
		if f, exists := flags[o]; exists && f.flag != 0 {
			if f.clear {
				flag &= ^f.flag
			} else {
				flag |= f.flag
			}
		} else if f, exists := propagationFlags[o]; exists && f != 0 {
			pgflag = append(pgflag, f)
		} else {
			data = append(data, o)
		}
	}
	return flag, pgflag, strings.Join(data, ",")
}

func (m *Mount) Apply(rootfs string) error {
	dest := filepath.Join(rootfs, m.Destination)
	if _, err := os.Lstat(dest); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(dest, 0755); err != nil {
			return err
		}
	}

	if err := unix.Mount(m.Source, dest, m.Device, m.Flags, m.Data); err != nil {
		return err
	}
	for _, pflag := range m.PropagationFlags {
		if err := unix.Mount("", dest, "", pflag, ""); err != nil {
			return err
		}
	}

	// bind mount won't change mount options, we need remount to make mount options effective.
	// first check that we have non-default options required before attempting a remount
	if m.Flags&^(unix.MS_REC|unix.MS_REMOUNT|unix.MS_BIND) != 0 {
		// only remount if unique mount options are set
		if err := unix.Mount(m.Source, dest, m.Device, m.Flags|unix.MS_REMOUNT, ""); err != nil {
			return err
		}
	}
	return nil
}

func ParseVolumn(s string) (*Mount, error) {
	opts := strings.SplitN(s, ":", 3)
	if len(opts) < 2 {
		return nil, ErrInvalidVolumn
	}
	mountOpts := []string{"bind", "rw", "exec"}
	if len(opts) == 3 {
		mountOpts = append(mountOpts, strings.Split(opts[2], ",")...)
	}
	flags, pgflags, data := parseMountOptions(mountOpts)
	return &Mount{
		Source:           opts[0],
		Destination:      opts[1],
		Flags:            flags,
		PropagationFlags: pgflags,
		Data:             data,
	}, nil
}
