package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/fancl20/bodman/manager"
	"github.com/fancl20/bodman/network"
	"github.com/google/uuid"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/urfave/cli/v2"
)

func newRunCommand() *cli.Command {
	return &cli.Command{
		Name:     "run",
		HideHelp: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name: "help",
			},
			&cli.StringSliceFlag{
				Name:  "dns",
				Value: cli.NewStringSlice("8.8.8.8"),
			},
			&cli.StringSliceFlag{
				Name:    "dns-option",
				Aliases: []string{"dns-opt"},
			},
			&cli.StringSliceFlag{
				Name: "dns-search",
			},
			&cli.StringSliceFlag{
				Name:    "env",
				Aliases: []string{"e"},
			},
			&cli.StringFlag{
				Name:    "hostname",
				Aliases: []string{"h"},
			},
			&cli.StringFlag{
				Name:    "network",
				Aliases: []string{"net"},
				Value:   "host",
			},
			&cli.StringSliceFlag{
				Name:    "publish",
				Aliases: []string{"p"},
			},
			&cli.BoolFlag{
				Name: "systemd-activation",
			},
			&cli.StringFlag{
				Name:    "user",
				Aliases: []string{"u"},
			},
			&cli.StringSliceFlag{
				Name:    "volume",
				Aliases: []string{"v"},
			},
			&cli.StringFlag{
				Name:    "workdir",
				Aliases: []string{"w"},
			},
		},
		Action: func(ctx *cli.Context) error {
			args := ctx.Args()
			containerID := uuid.New().String()
			containerDir, lock, err := manager.GetManager(ctx).ImageCheckout(args.First(), containerID)
			if err != nil {
				return err
			}
			// The lock won't be closed if we successfully call exec. This is
			// an intended behaviour so the container directory will be locked
			// until process exit.
			defer lock.Close()

			cfg, err := loadImageConfig(containerDir)
			if err != nil {
				return err
			}

			hostname := stringDefault(ctx.String("hostname"), strings.Split(containerID, "-")[0])

			networkConfig, err := network.NewNetwork(ctx, hostname, containerID)
			if err != nil {
				return fmt.Errorf("Create network config failed: %w", err)
			}
			networkConfigPath := filepath.Join(containerDir, "network.json")
			if err := network.Dump(networkConfigPath, networkConfig); err != nil {
				return fmt.Errorf("Dump network config failed: %w", err)
			}
			if err := networkConfig.Execute(); err != nil {
				return fmt.Errorf("Execute network config failed: %w", err)
			}

			if err := unix.Unshare(unix.CLONE_NEWIPC | unix.CLONE_NEWNS | unix.CLONE_NEWUTS); err != nil {
				return fmt.Errorf("Unshare namespaces failed: %w", err)
			}

			rootfs := filepath.Join(containerDir, "rootfs")
			mounts, err := parseMounts(ctx)
			if err != nil {
				return fmt.Errorf("Parse mounts failed: %w", err)
			}
			if err := prepareRootfs(rootfs, mounts); err != nil {
				return fmt.Errorf("Move root failed: %w", err)
			}

			cwd := stringDefault(ctx.String("workdir"), cfg.WorkingDir, "/")
			if err := unix.Chdir(cwd); err != nil {
				return fmt.Errorf("Chdir failed: %w", err)
			}

			if err := buildDNSResolve("/etc/resolv.conf", ctx.StringSlice("dns"), ctx.StringSlice("dns-search"), ctx.StringSlice("dns-option")); err != nil {
				return fmt.Errorf("Set dns failed: %w", err)
			}

			if err := unix.Sethostname([]byte(hostname)); err != nil {
				return fmt.Errorf("Sethostname failed: %w", err)
			}

			rawUser := stringDefault(ctx.String("user"), cfg.User)
			if rawUser != "" {
				uid, err := parseUser(rawUser)
				if err != nil {
					return err
				}
				unix.Setuid(uid)
			}

			cmd := stringSliceDefault(args.Tail(), cfg.Cmd)
			execArgs := append(cfg.Entrypoint, cmd...)
			if len(execArgs) == 0 {
				return fmt.Errorf("Empty exec args provided")
			}
			env := append(ctx.StringSlice("env"), cfg.Env...)
			executable, err := lookPath(execArgs[0], env)
			if err != nil {
				return err
			}
			if ctx.Bool("systemd-activation") {
				env = append(env, bypassSystemdActivation()...)
			}

			if err := unix.Exec(executable, execArgs, env); err != nil {
				return fmt.Errorf("Exec command failed: %w", err)
			}
			panic("unreachable")
		},
	}
}

func loadImageConfig(containerDir string) (*v1.ImageConfig, error) {
	f, err := os.Open(filepath.Join(containerDir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var img v1.Image
	if err := json.NewDecoder(f).Decode(&img); err != nil {
		return nil, err
	}
	return &img.Config, nil
}

func stringDefault(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func stringSliceDefault(ss ...[]string) []string {
	for _, s := range ss {
		if len(s) != 0 {
			return s
		}
	}
	return []string{}
}

func parseUser(s string) (int, error) {
	u, err := user.Lookup(s)
	if err == nil {
		s = u.Uid
	}
	uid, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("Invalid User: %v", s)
	}
	return int(uid), nil
}

func findExecutable(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if m := d.Mode(); !m.IsDir() && m&0111 != 0 {
		return nil
	}
	return os.ErrPermission
}

func lookPath(file string, envs []string) (string, error) {
	if strings.Contains(file, "/") {
		err := findExecutable(file)
		if err == nil {
			return file, nil
		}
		return "", fmt.Errorf("excutable not found")
	}
	var path string
	for _, e := range envs {
		if strings.HasPrefix(e, "PATH=") {
			path = strings.TrimPrefix(e, "PATH=")
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			dir = "."
		}
		path := filepath.Join(dir, file)
		if err := findExecutable(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("excutable not found")
}

func bypassSystemdActivation() []string {
	var envs []string
	for _, env := range []string{"LISTEN_PID", "LISTEN_FDS", "LISTEN_FDNAMES"} {
		if val, ok := os.LookupEnv(env); ok {
			envs = append(envs, fmt.Sprintf("%s=%s", env, val))
		}
	}
	return envs
}

func buildDNSResolve(path string, dns, dnsSearch, dnsOptions []string) error {
	content := bytes.NewBuffer(nil)
	if len(dnsSearch) > 0 {
		if searchString := strings.Join(dnsSearch, " "); strings.Trim(searchString, " ") != "." {
			if _, err := content.WriteString("search " + searchString + "\n"); err != nil {
				return err
			}
		}
	}
	for _, dns := range dns {
		if _, err := content.WriteString("nameserver " + dns + "\n"); err != nil {
			return err
		}
	}
	if len(dnsOptions) > 0 {
		if optsString := strings.Join(dnsOptions, " "); strings.Trim(optsString, " ") != "" {
			if _, err := content.WriteString("options " + optsString + "\n"); err != nil {
				return err
			}
		}
	}
	return ioutil.WriteFile(path, content.Bytes(), 0644)
}
