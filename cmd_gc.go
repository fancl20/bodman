package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	ostree "github.com/fancl20/ostree-go/pkg/otbuiltin"
	"github.com/urfave/cli/v2"
)

func newGCCommand() *cli.Command {
	return &cli.Command{
		Name:     "gc",
		HideHelp: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name: "help",
			},
		},
		Action: func(ctx *cli.Context) error {
			closed, err := getClosedContainerList(ctx)
			if err != nil {
				return err
			}
			for _, path := range closed {
				if err := os.RemoveAll(path); err != nil {
					return fmt.Errorf("Remove container dir failed: %s: %w", path, err)
				}
			}
			if _, err := pruneImages(ctx); err != nil {
				return err
			}
			return nil
		},
	}
}

func getClosedContainerList(ctx *cli.Context) ([]string, error) {
	base := ctx.String("base-directory")
	baseLock, err := tryLockFile(base, true)
	if err != nil {
		return nil, fmt.Errorf("Acquire base lock failed: %w", err)
	}
	defer baseLock.Close()

	containerDir := getContainersPath(base)
	containers, err := ioutil.ReadDir(containerDir)
	if err != nil {
		return nil, fmt.Errorf("List container directory failed: %w", err)
	}
	var closed []string
	for _, c := range containers {
		path := filepath.Join(containerDir, c.Name())
		l, err := tryLockFile(path, false)
		if err != nil {
			return nil, fmt.Errorf("Acquire container lock failed: %s: %w", path, err)
		}
		if l == nil {
			continue
		}
		l.Close()
		closed = append(closed, path)
	}
	return closed, nil
}

func pruneImages(ctx *cli.Context) (string, error) {
	base := ctx.String("base-directory")
	repo, err := openRepo(base)
	if err != nil {
		return "", err
	}
	pruneOpt := ostree.NewPruneOptions()
	pruneOpt.RefsOnly = true
	return repo.Prune(pruneOpt)
}
