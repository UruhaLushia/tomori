//go:build !windows

package fsutil

import (
	"errors"
	"syscall"
)

func IsNameTooLong(err error) bool { return errors.Is(err, syscall.ENAMETOOLONG) }
