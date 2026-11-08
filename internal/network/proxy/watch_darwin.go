//go:build darwin

package proxy

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func watchNativeProxy(ctx context.Context, changed func()) error {
	const path = "/Library/Preferences/SystemConfiguration/preferences.plist"
	fd, err := unix.Open(path, unix.O_EVTONLY, 0)
	if err != nil {
		return fmt.Errorf("open system proxy settings: %w", err)
	}
	defer unix.Close(fd)
	kqueue, err := unix.Kqueue()
	if err != nil {
		return err
	}
	defer unix.Close(kqueue)
	change := unix.Kevent_t{}
	unix.SetKevent(&change, fd, unix.EVFILT_VNODE, unix.EV_ADD|unix.EV_ENABLE|unix.EV_CLEAR)
	change.Fflags = unix.NOTE_WRITE | unix.NOTE_EXTEND | unix.NOTE_ATTRIB | unix.NOTE_RENAME | unix.NOTE_DELETE | unix.NOTE_REVOKE
	if _, err := unix.Kevent(kqueue, []unix.Kevent_t{change}, nil, nil); err != nil {
		return err
	}
	events := make([]unix.Kevent_t, 1)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := unix.Kevent(kqueue, nil, events, nil)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if n > 0 {
			if changed != nil {
				changed()
			}
			return nil
		}
	}
}
