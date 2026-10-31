package systeminfo

import (
	"context"
	"errors"

	"github.com/shirou/gopsutil/v4/mem"

	"github.com/UruhaLushia/tomori/locales"
)

type Memory struct {
	TotalMiB     int
	UsedMiB      int
	AvailableMiB int
}

func ReadMemory(ctx context.Context) (Memory, error) {
	stats, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return Memory{}, err
	}
	total := int(stats.Total >> 20)
	if total <= 0 {
		return Memory{}, errors.New(locales.CannotReadPhysicalMemoryInformation())
	}
	return Memory{TotalMiB: total, UsedMiB: min(total, int(stats.Used>>20)), AvailableMiB: min(total, int(stats.Available>>20))}, nil
}

// SuggestedMiB reserves 1 GiB for the system.
func (m Memory) SuggestedMiB() int {
	available := max(0, m.AvailableMiB-1024)
	value := available * 4 / 5
	return min(m.TotalMiB, max(512, min(value, 16384)))
}
