package app

import (
	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/launcher"
)

func (a *app) newService() *launcher.Service {
	files := a.cfg.Download.Workers
	connections := max(files, a.cfg.Download.Connections)
	if a.downloadBudget == nil {
		a.downloadBudget = download.NewBudget(files, connections)
	} else {
		a.downloadBudget.Configure(files, connections)
	}
	a.downloadBudget.SetSpeedLimit(int64(a.cfg.Download.SpeedLimitKiB) << 10)
	return launcher.New(*a.cfg, a.store, a.downloadBudget)
}
