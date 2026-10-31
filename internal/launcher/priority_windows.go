package launcher

import (
	"golang.org/x/sys/windows"

	"github.com/UruhaLushia/tomori/internal/config"
)

func setProcessPriority(pid int, priority config.ProcessPriority) error {
	if priority == config.PriorityNormal {
		return nil
	}
	class := uint32(windows.NORMAL_PRIORITY_CLASS)
	switch priority {
	case config.PriorityLow:
		class = windows.IDLE_PRIORITY_CLASS
	case config.PriorityBelowNormal:
		class = windows.BELOW_NORMAL_PRIORITY_CLASS
	case config.PriorityAboveNormal:
		class = windows.ABOVE_NORMAL_PRIORITY_CLASS
	case config.PriorityHigh:
		class = windows.HIGH_PRIORITY_CLASS
	case config.PriorityHighest:
		class = windows.REALTIME_PRIORITY_CLASS
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION, false, uint32(pid))
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(process) }()
	return windows.SetPriorityClass(process, class)
}
