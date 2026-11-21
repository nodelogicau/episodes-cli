//go:build darwin

package store

import (
	"os"

	"golang.org/x/sys/unix"
)

// syncData pushes a batch file's bytes to the device with a plain fsync.
// On macOS os.File.Sync is F_FULLFSYNC, which also flushes the drive's
// cache and costs milliseconds a file; a batch pays that once, in
// syncBarrier, instead of once per file.
func syncData(f *os.File) error {
	for {
		err := unix.Fsync(int(f.Fd()))
		if err != unix.EINTR {
			return err
		}
	}
}

// syncBarrier makes every file a batch wrote durable: an F_FULLFSYNC flushes
// the drive's cache, which holds what the plain fsyncs handed it.
func syncBarrier(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}
