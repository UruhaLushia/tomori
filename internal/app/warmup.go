package app

import (
	"runtime"
	"runtime/debug"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) reclaimStartupMemory() {
	a.workers.Go(func() {
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		select {
		case <-a.ctx.Done():
			return
		case <-timer.C:
		}
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		if stats.HeapSys-stats.HeapReleased > 16<<20 {
			debug.FreeOSMemory()
		}
	})
}

func (a *app) warmup() {
	ui.Render(func(c *ui.Context) {
		c.SetTheme(kit.Theme())
		ui.Column(c).Fill().Lang(c.Locale()).Children(func() {
			for _, size := range []float32{10, 11, 12, 13, 14} {
				kit.Preheat(c, locales.InstancesPage()+locales.Settings()+locales.AccountsPage()+locales.Network()+locales.Appearance()+locales.Logs()+locales.Java()+" 0123", size, false, false)
			}
			for _, size := range []float32{13, 14, 15, 16, 20, 24} {
				kit.Preheat(c, locales.InstancesPage()+locales.Settings()+locales.AccountsPage()+locales.Network()+locales.Appearance()+locales.Logs()+locales.Java()+" Minecraft", size, false, true)
			}
			for _, size := range []float32{11, 12} {
				kit.Preheat(c, locales.Value123456LaunchingMinecraft(), size, true, false)
			}
		})
	}, 480, 320, 1)
}
