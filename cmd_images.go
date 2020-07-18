package main

import (
	"fmt"

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
		Subcommands: []*cli.Command{
			newImageRemoveCommand(),
		},
		Action: func(ctx *cli.Context) error {
			branches, err := getBranchNames(ctx)
			if err != nil {
				return err
			}
			for _, branch := range branches {
				image, err := decodeImageFromBranch(branch)
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
	repo, err := openRepo(base)
	if err != nil {
		return nil, err
	}
	refs, err := repo.ListRefs()
	if err != nil {
		return nil, err
	}
	var ret []string
	for ref, _ := range refs {
		ret = append(ret, ref)
	}
	return ret, nil
}

func newImageRemoveCommand() *cli.Command {
	return &cli.Command{
		Name:     "rm",
		HideHelp: true,
		Action: func(ctx *cli.Context) error {
			imageName, err := parseImageNameToString(ctx.Args().First())
			if err != nil {
				return err
			}
			return deleteImageRef(ctx.String("base-directory"), imageName)
		},
	}
}
