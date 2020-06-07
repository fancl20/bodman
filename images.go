package main

import (
	"path/filepath"
	"strings"

	ostree "github.com/ostreedev/ostree-go/pkg/otbuiltin"
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
	if strings.Index(imageName, ":") != -1 {
		imageName = strings.ReplaceAll(imageName, ":", "/")
	} else {
		imageName += "/latest"
	}
	return "ociimage/" + imageName
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

func checkoutImage(base, imageName, containerID string) (string, error) {
	opts := ostree.NewCheckoutOptions()
	dst := filepath.Join(getContainersPath(base), containerID)
	if err := ostree.Checkout(getImagesPath(base), dst, getBranchName(imageName), opts); err != nil {
		return "", err
	}
	return dst, nil
}
