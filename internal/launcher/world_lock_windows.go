package launcher

import (
	"os"

	"golang.org/x/sys/windows"
)

const worldLockFlags = 0

func lockWorldFile(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, ^uint32(0), ^uint32(0), &windows.Overlapped{})
}
