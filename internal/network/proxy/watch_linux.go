//go:build linux

package proxy

import (
	"bufio"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

func watchNativeProxy(ctx context.Context, changed func()) error {
	env := envMap(linuxSessionEnv())
	if strings.Contains(strings.ToLower(env["XDG_CURRENT_DESKTOP"]), "kde") {
		return watchKDEProxy(ctx, changed, env)
	}
	return watchGSettings(ctx, changed)
}

func watchGSettings(ctx context.Context, changed func()) error {
	monitorCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	schemas := []string{"org.gnome.system.proxy", "org.gnome.system.proxy.http", "org.gnome.system.proxy.https", "org.gnome.system.proxy.ftp", "org.gnome.system.proxy.socks"}
	changedCh := make(chan struct{}, 1)
	for _, schema := range schemas {
		cmd := exec.CommandContext(monitorCtx, "gsettings", "monitor", schema)
		cmd.Env = linuxSessionEnv()
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		wg.Go(func() {
			if bufio.NewScanner(stdout).Scan() {
				select {
				case changedCh <- struct{}{}:
				case <-monitorCtx.Done():
				}
			}
			_ = cmd.Wait()
		})
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-changedCh:
		if changed != nil {
			changed()
		}
		return nil
	}
}

func watchKDEProxy(ctx context.Context, changed func(), env map[string]string) error {
	configHome := env["XDG_CONFIG_HOME"]
	if configHome == "" {
		configHome = filepath.Join(env["HOME"], ".config")
	}
	directory, filename := filepath.Dir(filepath.Join(configHome, "kioslaverc")), "kioslaverc"
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if _, err := unix.InotifyAddWatch(fd, directory, unix.IN_CLOSE_WRITE|unix.IN_MODIFY|unix.IN_MOVED_TO|unix.IN_CREATE|unix.IN_DELETE); err != nil {
		return err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = unix.Close(fd)
		case <-done:
		}
	}()
	buffer := make([]byte, unix.SizeofInotifyEvent+unix.NAME_MAX+1)
	for {
		n, err := unix.Read(fd, buffer)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if inotifyContains(buffer[:n], filename) {
			if changed != nil {
				changed()
			}
			return nil
		}
	}
}

func inotifyContains(data []byte, target string) bool {
	for offset := 0; offset+unix.SizeofInotifyEvent <= len(data); {
		event := (*unix.InotifyEvent)(unsafe.Pointer(&data[offset]))
		offset += unix.SizeofInotifyEvent
		length := int(event.Len)
		if offset+length > len(data) {
			return false
		}
		name := strings.TrimRight(string(data[offset:offset+length]), "\x00")
		offset += length
		if name == target {
			return true
		}
	}
	return false
}
