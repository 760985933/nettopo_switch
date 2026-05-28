//go:build windows

package main

import (
	"os"
)

// tryAcquireExclusive attempts to open a file for exclusive read-write access.
// On Windows, relies on OS sharing locks — opening with RDWR will fail if
// another process holds an exclusive handle.
func tryAcquireExclusive(filePath string) (*os.File, bool, error) {
	f, err := os.OpenFile(filePath, os.O_RDWR, 0)
	if err != nil {
		if isRolloutFileBusyError(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return f, true, nil
}

func releaseExclusiveLock(f *os.File) {
	if f == nil {
		return
	}
	f.Close()
}
