package main

import (
	"os"
	"runtime"

	"github.com/urfave/cli/v2"
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func main() {
	app := cli.NewApp()
	app.Name = "bodman"
	app.HideHelp = true
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name: "help",
		},
		&cli.StringFlag{
			Name:  "base-directory",
			Value: "/var/cache/bodman",
		},
		&cli.BoolFlag{
			Name: "quiet",
		},
	}
	app.Before = func(ctx *cli.Context) error {
		base := ctx.String("base-directory")
		if err := os.MkdirAll(getImagesPath(base), 0755); err != nil {
			return err
		}
		if err := os.MkdirAll(getContainersPath(base), 0755); err != nil {
			return err
		}
		return nil
	}
	app.Commands = []*cli.Command{
		newPullCommand(),
		newRunCommand(),
		newGCCommand(),
	}
	app.RunAndExitOnError()
}
