//go:build windows

package store

import (
	"os"

	"golang.org/x/sys/windows"
)

// Windows byte-range locks are mandatory, so the lock covers one byte far
// past the end of episodes.yaml: readers of the file's content are never
// blocked, and writers contend on the same byte.
const lockOffsetHigh = 0x7fffffff

func lockFile(f *os.File) error {
	ol := &windows.Overlapped{OffsetHigh: lockOffsetHigh}
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol)
}

func unlockFile(f *os.File) error {
	ol := &windows.Overlapped{OffsetHigh: lockOffsetHigh}
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}
