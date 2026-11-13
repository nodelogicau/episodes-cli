package store

import (
	"os"
	"path/filepath"
)

// Lock takes the workspace's exclusive advisory write lock, on episodes.yaml
// itself, and returns the function that releases it. Every writing verb
// holds it from minting until its index upsert completes; readers never take
// it. The operating system releases it when the process exits, however it
// exits, so a killed writer leaves nothing to clean up, and no lock file is
// ever added to the workspace.
func (w *Workspace) Lock() (unlock func(), err error) {
	f, err := os.OpenFile(filepath.Join(w.Root, ConfigFile), os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = unlockFile(f)
		_ = f.Close()
	}, nil
}
