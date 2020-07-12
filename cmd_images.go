package main

import (
	"fmt"

	ostree_b "github.com/fancl20/bodman/ostree"
	"github.com/urfave/cli/v2"
)

func newImageCommand() *cli.Command {
	return &cli.Command{
		Name:     "images",
		HideHelp: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name: "help",
			},
		},
		Action: func(ctx *cli.Context) error {
			branches, err := getBranchNames(ctx)
			if err != nil {
				return err
			}
			for _, branch := range branches {
				image, err := decodeImageNameFromBranch(branch)
				if err != nil {
					return fmt.Errorf("Decoding image name failed: %s: %w", branch, err)
				}
				fmt.Println(image)
			}
			return nil
		},
	}
}

func getBranchNames(ctx *cli.Context) ([]string, error) {
	base := ctx.String("base-directory")
	return ostree_b.ListRefs(getImagesPath(base))
}
