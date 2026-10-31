//go:build unix

package fsutil

import "golang.org/x/sys/unix"

const regularOpenFlags = unix.O_NONBLOCK | unix.O_NOFOLLOW
