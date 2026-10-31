package app

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/launcher"
)

type dialogState struct {
	install       page.InstallDialog
	accounts      page.Accounts
	pack          packDialogState
	pendingLaunch *pendingLaunch
}

type pendingLaunch struct {
	key    string
	target launcher.ServerTarget
}

func (a *app) openInstall(version string) {
	if a.tasksFull() || a.pathBusy {
		return
	}
	a.dialogs.install.Open(version)
	if len(a.versions) == 0 && !a.manifestLoading {
		a.loadManifest()
	}
}

func (a *app) dialogsView(c *ui.Context) {
	c.Key("install-dialog")
	a.dialogs.install.View(c, page.InstancesData{
		Space: a.installSpace.data, Loaders: a.loaderCatalog.data,
		Versions: a.versions, ManifestLoading: a.manifestLoading, ManifestError: a.manifestError,
		InstallDirectories: a.installDirectories(), Busy: a.tasksFull() || a.pathBusy,
	}, page.InstancesActions{AssessInstall: a.assessInstall, Loaders: a.loadLoaderVersions, Install: a.install})
	c.Key("account-dialogs")
	a.dialogs.accounts.Dialogs(c, a.accountData(), a.accountActions(c.Services()))
	if !a.dialogs.accounts.AddOpen() && !a.authBusy {
		a.dialogs.pendingLaunch = nil
	}
}

func (a *app) resumePendingLaunch() {
	pending := a.dialogs.pendingLaunch
	a.dialogs.pendingLaunch = nil
	if pending != nil && !a.authBusy {
		a.launchTarget(pending.key, pending.target)
	}
}
