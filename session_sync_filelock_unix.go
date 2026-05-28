//go:build !windows

package main

import (
	"os"
	"syscall"
)

// tryAcquireExclusive attempts to open a file for exclusive read-write access.
// Returns a handle and true on success, or nil and false if the file is locked.
func tryAcquireExclusive(filePath string) (*os.File, bool, error) {
	f, err := os.OpenFile(filePath, os.O_RDWR, 0)
	if err != nil {
		if isRolloutFileBusyError(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()
		return nil, false, nil
	}
	return f, true, nil
}

func releaseExclusiveLock(f *os.File) {
	if f == nil {
		return
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	f.Close()
}
