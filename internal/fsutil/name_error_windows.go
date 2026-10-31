//go:build windows

package fsutil

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

func IsNameTooLong(err error) bool {
	return errors.Is(err, syscall.ENAMETOOLONG) || errors.Is(err, windows.ERROR_FILENAME_EXCED_RANGE)
}
