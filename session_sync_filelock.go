package main

import (
	"os"
	"runtime"
	"strings"
	"syscall"
)

// isRolloutFileBusyError checks if an error indicates the file is locked by another process.
func isRolloutFileBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "ebusy") ||
		strings.Contains(msg, "resource busy or locked") ||
		strings.Contains(msg, "being used by another process") ||
		strings.Contains(msg, "currently in use") ||
		strings.Contains(msg, "eperm")
}

// tryAcquireExclusive attempts to open a file for exclusive read-write access.
// Returns a handle and true on success, or nil and false if the file is locked.
func tryAcquireExclusive(filePath string) (*os.File, bool, error) {
	// On Unix, try flock-based exclusive lock
	if runtime.GOOS != "windows" {
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
			if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
				return nil, false, nil
			}
			return nil, false, nil
		}
		return f, true, nil
	}

	// On Windows, try opening with exclusive access
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
	if runtime.GOOS != "windows" {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}
	f.Close()
}

// findLockedFiles returns paths that are currently locked (unable to acquire exclusive access).
func findLockedFiles(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	locked := make([]string, 0)
	for _, p := range paths {
		f, ok, _ := tryAcquireExclusive(p)
		if !ok {
			locked = append(locked, p)
		}
		if f != nil {
			releaseExclusiveLock(f)
		}
	}
	return locked
}

// splitLockedChanges separates changes into writable and locked slices.
func splitLockedChanges(changes []syncRolloutChange) (writable []syncRolloutChange, locked []syncRolloutChange) {
	paths := make([]string, len(changes))
	for i, c := range changes {
		paths[i] = c.Path
	}
	lockedSet := make(map[string]bool, len(paths))
	for _, p := range findLockedFiles(paths) {
		lockedSet[p] = true
	}

	for _, c := range changes {
		if lockedSet[c.Path] {
			locked = append(locked, c)
		} else {
			writable = append(writable, c)
		}
	}
	return
}
