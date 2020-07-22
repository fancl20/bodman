package main

import (
	"github.com/fancl20/bodman/manager"
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
			m := manager.GetManager(ctx)
			if _, err := m.ContainerPrune(); err != nil {
				return err
			}
			if _, err := m.ImagePrune(); err != nil {
				return err
			}
			return nil
		},
	}
}
