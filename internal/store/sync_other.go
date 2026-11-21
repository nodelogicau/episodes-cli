//go:build !darwin

package store

import "os"

// syncData is os.File.Sync, which is already a full sync off macOS.
func syncData(f *os.File) error { return f.Sync() }

// syncBarrier has nothing to add where each file's sync is already full.
func syncBarrier(string) error { return nil }
