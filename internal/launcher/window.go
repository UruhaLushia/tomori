package launcher

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
)

// SetWindowedDisplaySizes supplies a UI-thread snapshot of XWayland display
// sizes. No display APIs are called from launch workers.
func (s *Service) SetWindowedDisplaySizes(sizes [][2]int) {
	s.windowedDisplaySizes = slices.Clone(sizes)
}

func (c *LaunchConfig) avoidImplicitFullscreen(sizes [][2]int) {
	if c.FullScreen || c.Width <= 0 || c.Height <= 0 {
		return
	}
	// xwayland-satellite treats an exact display-sized window as fullscreen,
	// even without a request from GLFW. Keep the requested size to within one
	// pixel in the usual case, without changing the saved game settings.
	for c.Height > 1 && slices.Contains(sizes, [2]int{c.Width, c.Height}) {
		c.Height--
	}
}

// Minecraft also restores fullscreen from options.txt when the command line
// omits --fullscreen. Synchronize that value before it reads its options.
func setGameFullscreen(gameDir string, fullscreen bool) error {
	path := filepath.Join(gameDir, "options.txt")
	path, err := filepath.EvalSymlinks(path)
	if errors.Is(err, os.ErrNotExist) {
		// A new game uses windowed mode by default; --fullscreen handles opt-in.
		return nil
	}
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	value := []byte("fullscreen:" + strconv.FormatBool(fullscreen))
	lines := bytes.Split(raw, []byte("\n"))
	found := false
	for i, line := range lines {
		if !bytes.HasPrefix(line, []byte("fullscreen:")) {
			continue
		}
		found = true
		lines[i] = bytes.Clone(value)
		if bytes.HasSuffix(line, []byte("\r")) {
			lines[i] = append(lines[i], '\r')
		}
	}
	updated := bytes.Join(lines, []byte("\n"))
	if !found {
		newline := []byte("\n")
		if bytes.Contains(raw, []byte("\r\n")) {
			newline = []byte("\r\n")
		}
		if len(updated) > 0 && !bytes.HasSuffix(updated, []byte("\n")) {
			updated = append(updated, newline...)
		}
		updated = append(updated, value...)
		updated = append(updated, newline...)
	}
	if bytes.Equal(raw, updated) {
		return nil
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tomori-options-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err := f.Chmod(info.Mode().Perm()); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(updated); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
