//go:build unix

package fsutil

import (
	"os"
	"syscall"
)

// Lock takes an exclusive advisory lock on f, blocking until it is available.
func Lock(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) }

// Unlock releases a lock taken with Lock.
func Unlock(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
