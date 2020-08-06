package main

import (
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
		&cli.StringFlag{
			Name:  "cni-config-dir",
			Value: "/etc/cni/net.d/",
		},
		&cli.StringSliceFlag{
			Name: "cni-plugin-dir",
			Value: cli.NewStringSlice(
				"/usr/libexec/cni",
				"/usr/lib/cni",
				"/usr/local/lib/cni",
				"/opt/cni/bin"),
		},
	}
	app.Commands = []*cli.Command{
		newGCCommand(),
		newImageCommand(),
		newPullCommand(),
		newRunCommand(),
	}
	app.RunAndExitOnError()
}
