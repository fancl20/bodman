package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/storage/pkg/archive"
	"github.com/fancl20/bodman/manager"
	digest "github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/urfave/cli/v2"
)

func newPullCommand() *cli.Command {
	return &cli.Command{
		Name:     "pull",
		HideHelp: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name: "help",
			},
		},
		Action: func(ctx *cli.Context) error {
			args := ctx.Args()
			if args.Len() != 1 {
				return fmt.Errorf("Exactly one arguments expected")
			}
			tempDir, err := ioutil.TempDir("", "bodman-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tempDir)

			// Copy image from remote source
			// dstName := fmt.Sprintf("oci:%s/image", tempDir)
			output := os.Stdout
			if ctx.IsSet("quiet") {
				output = nil
			}
			if err := copyImage(ctx.Context, args.Get(0), tempDir, output); err != nil {
				return err
			}

			// Unpack image to the temp build directory
			imageDir := filepath.Join(tempDir, "image")
			entryManifestDigest, err := getImageEntrypointDigest(filepath.Join(imageDir, "index.json"))
			if err != nil {
				return err
			}
			blobsDir := filepath.Join(imageDir, "blobs")
			entryManifest, err := getManifestFromDigest(blobsDir, entryManifestDigest)
			if err != nil {
				return err
			}
			buildDir := filepath.Join(tempDir, "build")
			rootfsDir := filepath.Join(buildDir, "rootfs")
			if err := os.Mkdir(buildDir, 0755); err != nil {
				return err
			}
			for _, l := range entryManifest.Layers {
				if err := applyLayer(digestToPath(blobsDir, l.Digest), rootfsDir); err != nil {
					return err
				}
			}
			configFilePath := digestToPath(blobsDir, entryManifest.Config.Digest)
			if err := copyFile(configFilePath, filepath.Join(buildDir, "manifest.json")); err != nil {
				return err
			}

			// Commit image
			if err := manager.GetManager(ctx).ImageCommit(args.First(), buildDir); err != nil {
				return err
			}
			return nil
		},
	}
}

func copyImage(ctx context.Context, srcName, dstName string, stdout io.Writer) error {
	policy, err := signature.DefaultPolicy(nil)
	if err != nil {
		return fmt.Errorf("Error creating trust policy: %w", err)
	}
	policyContext, err := signature.NewPolicyContext(policy)
	if err != nil {
		return fmt.Errorf("Error loading trust policy: %w", err)
	}
	defer policyContext.Destroy()

	srcRef, err := alltransports.ParseImageName(fmt.Sprintf("docker://%s", srcName))
	if err != nil {
		return fmt.Errorf("Invalid source name %s: %w", srcName, err)
	}
	dstName = fmt.Sprintf("oci:%s/image", dstName)
	destRef, err := alltransports.ParseImageName(dstName)
	if err != nil {
		return fmt.Errorf("Invalid destination name %s: %w", dstName, err)
	}

	imageListSelection := copy.CopySystemImage

	_, err = copy.Image(ctx, policyContext, destRef, srcRef, &copy.Options{
		ReportWriter:       stdout,
		ImageListSelection: imageListSelection,
	})
	return err
}

func digestToPath(base string, d digest.Digest) string {
	path := append([]string{base}, strings.Split(d.String(), ":")...)
	return filepath.Join(path...)
}

func getImageEntrypointDigest(indexPath string) (digest.Digest, error) {
	indexFile, err := os.Open(indexPath)
	if err != nil {
		return "", err
	}
	defer indexFile.Close()
	var index v1.Index
	if err := json.NewDecoder(indexFile).Decode(&index); err != nil {
		return "", err
	}
	if len(index.Manifests) != 1 {
		return "", fmt.Errorf("Exactly one Manifest is expected in index.json")
	}
	return index.Manifests[0].Digest, nil
}

func getManifestFromDigest(base string, d digest.Digest) (*v1.Manifest, error) {
	manifestFile, err := os.Open(digestToPath(base, d))
	if err != nil {
		return nil, err
	}
	defer manifestFile.Close()
	var manifest v1.Manifest
	if err := json.NewDecoder(manifestFile).Decode(&manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func applyLayer(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	_, err = archive.ApplyLayer(dst, f)
	return err
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	stat, err := srcFile.Stat()
	if err != nil {
		return err
	}
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, stat.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	return err
}
