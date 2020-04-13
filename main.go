package main

import (
	"github.com/urfave/cli/v2"
)

func main() {
	app := cli.NewApp()
	app.Name = "bodman"

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:  "base-directory",
			Value: "/var/cache/bodman",
		},
		&cli.StringFlag{
			Name:  "runtime",
			Value: "crun",
		},
		&cli.BoolFlag{
			Name: "quiet",
		},
	}
	app.Commands = []*cli.Command{
		newPullCommand(),
		// 	runCommand,
		// 	forkCommand,
	}
	app.RunAndExitOnError()
}
