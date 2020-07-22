package manager

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/containers/image/v5/transports/alltransports"
	ostree "github.com/fancl20/ostree-go/pkg/otbuiltin"
	"github.com/urfave/cli/v2"
	"golang.org/x/sys/unix"
)

func getImagesPath(base string) string {
	return filepath.Join(base, "images")
}

func getContainersPath(base string) string {
	return filepath.Join(base, "containers")
}

func encodeBranchFromImage(image string) (string, error) {
	image, err := normalizeImageName(image)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString([]byte(image)), nil
}

func decodeImageFromBranch(branch string) (string, error) {
	image, err := base64.RawURLEncoding.DecodeString(branch)
	if err != nil {
		return "", err
	}
	return string(image), nil
}

func normalizeImageName(imageName string) (string, error) {
	image, err := alltransports.ParseImageName(fmt.Sprintf("docker://%s", imageName))
	if err != nil {
		return "", err
	}
	return image.DockerReference().String(), nil
}

func tryLockFile(path string, blocking bool) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_DIRECTORY|unix.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	blockingFlag := 0
	if !blocking {
		blockingFlag = unix.LOCK_NB
	}
	if err := unix.Flock(fd, unix.LOCK_EX|blockingFlag); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, nil
		}
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

type Manager struct {
	base string
	repo *ostree.Repo
	err  error
}

var cachedManager *Manager

func GetManager(ctx *cli.Context) *Manager {
	if cachedManager != nil {
		return cachedManager
	}
	base := ctx.String("base-directory")

	var err error
	if cachedManager, err = func() (*Manager, error) {
		if err := os.MkdirAll(getImagesPath(base), 0755); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(getContainersPath(base), 0755); err != nil {
			return nil, err
		}
		opts := ostree.NewInitOptions()
		opts.Mode = "bare-user"
		repoPath := getImagesPath(base)
		if _, err := ostree.Init(repoPath, opts); err != nil {
			return nil, err
		}
		repo, err := ostree.OpenRepo(repoPath)
		if err != nil {
			return nil, err
		}
		return &Manager{
			base: base,
			repo: repo,
		}, nil
	}(); err != nil {
		cachedManager = &Manager{err: err}
	}
	return cachedManager
}

func (m *Manager) ImageCommit(image, dir string) error {
	if m.err != nil {
		return m.err
	}
	if _, err := m.repo.PrepareTransaction(); err != nil {
		return err
	}
	branch, err := encodeBranchFromImage(image)
	if err != nil {
		return err
	}
	if _, err := m.repo.Commit(dir, branch, ostree.NewCommitOptions()); err != nil {
		return err
	}
	if _, err := m.repo.CommitTransaction(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) ImageCheckout(image, container string) (string, *os.File, error) {
	if m.err != nil {
		return "", nil, m.err
	}
	baseLock, err := tryLockFile(m.base, true)
	if err != nil {
		return "", nil, fmt.Errorf("Acquire base lock failed: %w", err)
	}
	defer baseLock.Close()
	opts := ostree.NewCheckoutOptions()
	opts.UserMode = true
	opts.RequireHardlinks = true
	dst := filepath.Join(getContainersPath(m.base), container)
	branch, err := encodeBranchFromImage(image)
	if err != nil {
		return "", nil, m.err
	}
	if err := ostree.Checkout(getImagesPath(m.base), dst, branch, opts); err != nil {
		return "", nil, fmt.Errorf("Checkout image failed: %w", err)
	}
	containerLock, err := tryLockFile(dst, true)
	if err != nil {
		return "", nil, fmt.Errorf("Acquire container lock failed: %w", err)
	}
	return dst, containerLock, nil
}

func (m *Manager) ImageDelete(image string) error {
	if m.err != nil {
		return m.err
	}
	if _, err := m.repo.PrepareTransaction(); err != nil {
		return err
	}
	branch, err := encodeBranchFromImage(image)
	if err != nil {
		return err
	}
	m.repo.TransactionSetRef("", branch, "")
	if _, err := m.repo.CommitTransaction(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) ImageList() ([]string, error) {
	refs, err := m.repo.ListRefs()
	if err != nil {
		return nil, err
	}
	var ret []string
	for ref, _ := range refs {
		image, err := decodeImageFromBranch(ref)
		if err != nil {
			return nil, fmt.Errorf("Decode image name failed: %s: %w", ref, err)
		}
		ret = append(ret, image)
	}
	return ret, nil

}

func (m *Manager) ImagePrune() (string, error) {
	if m.err != nil {
		return "", m.err
	}
	pruneOpt := ostree.NewPruneOptions()
	pruneOpt.RefsOnly = true
	return m.repo.Prune(pruneOpt)
}

func (m *Manager) ContainerPrune() ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	baseLock, err := tryLockFile(m.base, true)
	if err != nil {
		return nil, fmt.Errorf("Acquire base lock failed: %w", err)
	}
	defer baseLock.Close()

	containerDir := getContainersPath(m.base)
	containers, err := ioutil.ReadDir(containerDir)
	if err != nil {
		return nil, fmt.Errorf("List container directory failed: %w", err)
	}

	var stopped []string
	for _, c := range containers {
		path := filepath.Join(containerDir, c.Name())
		l, err := tryLockFile(path, false)
		if err != nil {
			return stopped, fmt.Errorf("Acquire container lock failed: %s: %w", path, err)
		}
		if l == nil {
			continue
		}
		l.Close()

		if err := os.RemoveAll(path); err != nil {
			return stopped, fmt.Errorf("Remove container dir failed: %s: %w", path, err)
		}
		stopped = append(stopped, path)
	}
	return stopped, nil
}
