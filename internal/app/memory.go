package app

import (
	"time"

	"github.com/UruhaLushia/tomori/internal/systeminfo"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) loadMemory() {
	a.workers.Go(func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			memory, err := systeminfo.ReadMemory(a.ctx)
			a.win.Update(func() {
				if a.ctx.Err() != nil {
					return
				}
				if err != nil {
					if a.memoryError == "" {
						a.logf(locales.MemoryReadFailed(), err)
					}
					a.memoryError = locales.FailedToReadMemoryInformation()
					return
				}
				a.memory, a.memoryError = memory, ""
				if a.cfg.Game.MemoryMiB > memory.TotalMiB {
					a.cfg.Game.MemoryMiB = memory.TotalMiB
					a.saveConfig()
				}
			})
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
}
