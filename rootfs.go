package main

import (
	"fmt"
	"strings"

	"github.com/fancl20/bodman/mount"
	"golang.org/x/sys/unix"
)

func prepareRootfs(rootfs string, mounts []*mount.Mount) error {
	if err := prepareRoot(rootfs); err != nil {
		return fmt.Errorf("Prepare root failed: %w", err)
	}
	for _, m := range mounts {
		if err := m.Apply(rootfs); err != nil {
			return fmt.Errorf("Apply %+v failed: %w", m, err)
		}
	}
	if err := pivotRoot(rootfs); err != nil {
		return fmt.Errorf("Pivot root failed: %w", err)
	}
	return nil
}

func prepareRoot(rootfs string) error {
	if err := unix.Mount("", "/", "", unix.MS_SLAVE|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("Make old root private failed: %w", err)
	}
	if err := rootfsParentMountPrivate(rootfs); err != nil {
		return fmt.Errorf("Make rootfs parent private failed: %w", err)
	}
	if err := unix.Mount(rootfs, rootfs, "bind", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("Mount rootfs failed: %w", err)
	}
	if err := unix.Chdir(rootfs); err != nil {
		return fmt.Errorf("Chdir to rootfs failed: %w", err)
	}
	return nil
}

// pivotRoot will call pivot_root such that rootfs becomes the new root
// filesystem, and everything else is cleaned up.
func pivotRoot(rootfs string) error {
	// While the documentation may claim otherwise, pivot_root(".", ".") is
	// actually valid. What this results in is / being the new root but
	// /proc/self/cwd being the old root. Since we can play around with the cwd
	// with pivot_root this allows us to pivot without creating directories in
	// the rootfs. Shout-outs to the LXC developers for giving us this idea.

	oldroot, err := unix.Open("/", unix.O_DIRECTORY|unix.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(oldroot)

	newroot, err := unix.Open(rootfs, unix.O_DIRECTORY|unix.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(newroot)

	// Change to the new root so that the pivot_root actually acts on it.
	if err := unix.Fchdir(newroot); err != nil {
		return err
	}

	if err := unix.PivotRoot(".", "."); err != nil {
		return fmt.Errorf("pivot_root %s", err)
	}

	// Currently our "." is oldroot (according to the current kernel code).
	// However, purely for safety, we will fchdir(oldroot) since there isn't
	// really any guarantee from the kernel what /proc/self/cwd will be after a
	// pivot_root(2).

	if err := unix.Fchdir(oldroot); err != nil {
		return err
	}

	// Make oldroot rslave to make sure our unmounts don't propagate to the
	// host (and thus bork the machine). We don't use rprivate because this is
	// known to cause issues due to races where we still have a reference to a
	// mount while a process in the host namespace are trying to operate on
	// something they think has no mounts (devicemapper in particular).
	if err := unix.Mount("", ".", "", unix.MS_SLAVE|unix.MS_REC, ""); err != nil {
		return err
	}
	// Preform the unmount. MNT_DETACH allows us to unmount /proc/self/cwd.
	if err := unix.Unmount(".", unix.MNT_DETACH); err != nil {
		return err
	}

	// Switch back to our shiny new root.
	if err := unix.Chdir("/"); err != nil {
		return fmt.Errorf("chdir / %s", err)
	}
	return nil
}

// Get the parent mount point of directory passed in as argument. Also return
// optional fields.
func getParentMount(rootfs string) (string, string, error) {
	mi, err := mount.GetMounts(mount.ParentsFilter(rootfs))
	if err != nil {
		return "", "", err
	}
	if len(mi) < 1 {
		return "", "", fmt.Errorf("could not find parent mount of %s", rootfs)
	}

	// find the longest mount point
	var idx, maxlen int
	for i := range mi {
		if len(mi[i].Mountpoint) > maxlen {
			maxlen = len(mi[i].Mountpoint)
			idx = i
		}
	}
	return mi[idx].Mountpoint, mi[idx].Optional, nil
}

func rootfsParentMountPrivate(rootfs string) error {
	parentMount, optionalOpts, err := getParentMount(rootfs)
	if err != nil {
		return err
	}

	sharedMount := false
	for _, opt := range strings.Split(optionalOpts, " ") {
		if strings.HasPrefix(opt, "shared:") {
			sharedMount = true
			break
		}
	}

	// Make parent mount PRIVATE if it was shared. It is needed for two
	// reasons. First of all pivot_root() will fail if parent mount is
	// shared. Secondly when we bind mount rootfs it will propagate to
	// parent namespace and we don't want that to happen.
	if sharedMount {
		return unix.Mount("", parentMount, "", unix.MS_PRIVATE, "")
	}

	return nil
}
