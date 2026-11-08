//go:build windows

package proxy

import (
	"context"
	"fmt"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func watchNativeProxy(ctx context.Context, changed func()) error {
	paths := []string{
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings\Connections`,
	}
	keys := make([]registry.Key, 0, len(paths))
	for _, path := range paths {
		key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.NOTIFY)
		if err != nil {
			for _, opened := range keys {
				_ = opened.Close()
			}
			return fmt.Errorf("open proxy registry: %w", err)
		}
		keys = append(keys, key)
	}
	defer func() {
		for _, key := range keys {
			_ = key.Close()
		}
	}()
	handles := make([]windows.Handle, 0, len(keys)+1)
	for _, key := range keys {
		event, err := windows.CreateEvent(nil, 0, 0, nil)
		if err != nil {
			closeWindowsHandles(handles)
			return err
		}
		handles = append(handles, event)
		if err := windows.RegNotifyChangeKeyValue(windows.Handle(key), false, windows.REG_NOTIFY_CHANGE_NAME|windows.REG_NOTIFY_CHANGE_LAST_SET, event, true); err != nil {
			closeWindowsHandles(handles)
			return err
		}
	}
	cancelEvent, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		closeWindowsHandles(handles)
		return err
	}
	handles = append(handles, cancelEvent)
	defer closeWindowsHandles(handles)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = windows.SetEvent(cancelEvent)
		case <-done:
		}
	}()
	index, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
	if err != nil {
		return err
	}
	if index == windows.WAIT_OBJECT_0+uint32(len(handles)-1) {
		return ctx.Err()
	}
	if changed != nil {
		changed()
	}
	return nil
}

func closeWindowsHandles(handles []windows.Handle) {
	for _, handle := range handles {
		_ = windows.CloseHandle(handle)
	}
}
