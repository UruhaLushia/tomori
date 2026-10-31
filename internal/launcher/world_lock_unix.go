//go:build unix

package launcher

import (
	"os"

	"golang.org/x/sys/unix"
)

const worldLockFlags = unix.O_NONBLOCK | unix.O_NOFOLLOW

func lockWorldFile(f *os.File) error {
	return unix.FcntlFlock(f.Fd(), unix.F_SETLK, &unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: 0, Len: 0})
}
