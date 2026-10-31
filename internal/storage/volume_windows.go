package storage

import (
	"errors"
	"strings"

	"golang.org/x/sys/windows"
)

func diskFull(err error) bool {
	return errors.Is(err, windows.ERROR_DISK_FULL) || errors.Is(err, windows.ERROR_HANDLE_DISK_FULL)
}

func volumeKey(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	var buffer [windows.MAX_PATH + 1]uint16
	if err := windows.GetVolumePathName(name, &buffer[0], uint32(len(buffer))); err != nil {
		return "", err
	}
	var volume [windows.MAX_PATH + 1]uint16
	if err := windows.GetVolumeNameForVolumeMountPoint(&buffer[0], &volume[0], uint32(len(volume))); err == nil {
		return strings.ToLower(windows.UTF16ToString(volume[:])), nil
	}
	return strings.ToLower(windows.UTF16ToString(buffer[:])), nil
}
