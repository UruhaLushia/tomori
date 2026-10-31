//go:build linux || darwin

package storage

import (
	"errors"
	"os"
	"strconv"
	"syscall"
)

func diskFull(err error) bool {
	return errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT)
}

func volumeKey(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(uint64(info.Sys().(*syscall.Stat_t).Dev), 10), nil
}
