package main

import (
	"fmt"
	"strings"

	"github.com/fancl20/bodman/manager"
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
			{
				Name:     "rm",
				HideHelp: true,
				Action: func(ctx *cli.Context) error {
					return manager.GetManager(ctx).ImageDelete(ctx.Args().First())
				},
			},
		},
		Action: func(ctx *cli.Context) error {
			images, err := manager.GetManager(ctx).ImageList()
			if err != nil {
				return err
			}
			fmt.Println(strings.Join(images, "\n"))
			return nil
		},
	}
}
