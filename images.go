package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	ostree "github.com/ostreedev/ostree-go/pkg/otbuiltin"
	"golang.org/x/sys/unix"
)

func getImagesPath(base string) string {
	return filepath.Join(base, "images")
}

func getContainersPath(base string) string {
	return filepath.Join(base, "containers")
}

func openRepo(base string) (*ostree.Repo, error) {
	opts := ostree.NewInitOptions()
	opts.Mode = "bare-user"
	repoPath := getImagesPath(base)
	if _, err := ostree.Init(repoPath, opts); err != nil {
		return nil, err
	}
	return ostree.OpenRepo(repoPath)
}

func getBranchName(imageName string) string {
	if strings.Index(imageName, ":") == -1 {
		imageName += ":latest"
	}
	return base64.RawURLEncoding.EncodeToString([]byte(imageName))
}

func decodeImageNameFromBranch(branch string) (string, error) {
	image, err := base64.RawURLEncoding.DecodeString(branch)
	if err != nil {
		return "", err
	}
	return string(image), nil
}

func parseImageNameToString(rawImage string) (string, error) {
	image, err := parseImageName(rawImage)
	if err != nil {
		return "", err
	}
	return image.DockerReference().String(), nil
}

func parseImageName(rawImage string) (types.ImageReference, error) {
	return alltransports.ParseImageName(fmt.Sprintf("docker://%s", rawImage))
}

func commitImage(base, imageName, buildDir string) error {
	repo, err := openRepo(base)
	if err != nil {
		return err
	}
	if _, err := repo.PrepareTransaction(); err != nil {
		return err
	}
	if _, err := repo.Commit(buildDir, getBranchName(imageName), ostree.NewCommitOptions()); err != nil {
		return err
	}
	if _, err := repo.CommitTransaction(); err != nil {
		return err
	}
	return nil
}

func checkoutImage(base, imageName, containerID string) (string, *os.File, error) {
	baseLock, err := tryLockFile(base, true)
	if err != nil {
		return "", nil, fmt.Errorf("Acquire base lock failed: %w", err)
	}
	defer baseLock.Close()

	opts := ostree.NewCheckoutOptions()
	dst := filepath.Join(getContainersPath(base), containerID)
	if err := ostree.Checkout(getImagesPath(base), dst, getBranchName(imageName), opts); err != nil {
		return "", nil, fmt.Errorf("Checkout image failed: %w", err)
	}
	containerLock, err := tryLockFile(dst, true)
	if err != nil {
		return "", nil, fmt.Errorf("Acquire container lock failed: %w", err)
	}
	return dst, containerLock, nil
}

func tryLockFile(path string, blocking bool) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_DIRECTORY|unix.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	blockingFlag := 0
	if !blocking {
		blockingFlag = unix.LOCK_NB
	}
	if err := unix.Flock(fd, unix.LOCK_EX|blockingFlag); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, nil
		}
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
