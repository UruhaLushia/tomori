//go:build linux || darwin

package launcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"golang.org/x/sys/unix"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/locales"
)

func setProcessPriority(pid int, priority config.ProcessPriority) error {
	if priority == config.PriorityNormal {
		return nil
	}
	nice := 0
	switch priority {
	case config.PriorityLow:
		nice = 19
	case config.PriorityBelowNormal:
		nice = 10
	case config.PriorityAboveNormal:
		nice = -5
	case config.PriorityHigh:
		nice = -10
	case config.PriorityHighest:
		nice = -20
	}
	if err := unix.Setpriority(unix.PRIO_PROCESS, pid, nice); err != nil {
		return err
	}
	if runtime.GOOS != "linux" {
		return nil
	}
	// Linux nice values belong to threads. Cover threads the JVM has already
	// created as well as those created while the first pass is in progress.
	seen := map[int]bool{pid: true}
	for range 8 {
		tasks, err := os.ReadDir(filepath.Join("/proc", strconv.Itoa(pid), "task"))
		if err != nil {
			return err
		}
		changed := false
		for _, task := range tasks {
			tid, err := strconv.Atoi(task.Name())
			if err != nil || tid <= 0 || seen[tid] {
				continue
			}
			if err := unix.Setpriority(unix.PRIO_PROCESS, tid, nice); err != nil && !errors.Is(err, unix.ESRCH) {
				return fmt.Errorf(locales.Thread(), tid, err)
			}
			seen[tid], changed = true, true
		}
		if !changed {
			return nil
		}
	}
	return nil
}
