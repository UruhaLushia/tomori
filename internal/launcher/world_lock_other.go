//go:build !unix && !windows

package launcher

import (
	"errors"
	"os"
)

const worldLockFlags = 0

func lockWorldFile(*os.File) error { return errors.New("world locks are unavailable") }
