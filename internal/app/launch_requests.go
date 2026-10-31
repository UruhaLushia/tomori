package app

import (
	"slices"

	"github.com/UruhaLushia/tomori/internal/launchrequest"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) receiveArguments(args []string, directory string) {
	a.showLauncher()
	request, err := launchrequest.Parse(args, directory)
	if err != nil {
		a.notify(locales.LaunchFailed(), err.Error(), true)
		return
	}
	if !request.URL {
		a.receiveLaunchRequest(request)
	}
}

func (a *app) receiveURL(raw string) {
	a.showLauncher()
	request, err := launchrequest.ParseURL(raw)
	if err != nil {
		a.notify(locales.LaunchFailed(), err.Error(), true)
		return
	}
	a.receiveLaunchRequest(request)
}

func (a *app) receiveLaunchRequest(request launchrequest.Request) {
	if request.Action != launchrequest.Launch && request.Action != launchrequest.Open {
		return
	}
	if slices.Contains(a.launchRequests, request) {
		return
	}
	if len(a.launchRequests) == 8 {
		a.notify(locales.TooManyRequests(), "", true)
		return
	}
	a.launchRequests = append(a.launchRequests, request)
	a.drainLaunchRequests()
}

func (a *app) drainLaunchRequests() {
	if a.win == nil || !a.instancesReady || a.javaLoading || a.packRecovery.data.Busy || a.ctx.Err() != nil {
		return
	}
	requests := a.launchRequests
	a.launchRequests = nil
	for _, request := range requests {
		instance, err := request.Resolve(a.installedList)
		if err != nil {
			a.notify(locales.LaunchFailed(), err.Error(), true)
			continue
		}
		if a.pathBusy || a.tasksFull() {
			a.notify(locales.AnotherOperationIsInProgress(), "", true)
			continue
		}
		if request.Action == launchrequest.Open {
			if selected, ok := a.pages.instances.Selected(a.installedList); ok && selected.Key() != instance.Key() {
				a.resetInstanceContent()
				a.closeResources()
				a.releaseServers()
			}
			a.goPage(pageInstances) // nav:request
			a.pages.instances.Open(instance.Key())
			continue
		}
		if a.instanceActive(instance.Key()) {
			continue
		}
		if a.instanceInUse(instance.Key()) {
			a.notify(locales.AnotherOperationIsInProgress(), "", true)
			continue
		}
		if !instance.CanLaunch() {
			a.notify(locales.LaunchFailed(), locales.UnsupportedInstanceFormatAlt(), true)
			continue
		}
		a.goPage(pageHome) // nav:request
		a.selectVersion(instance.Key())
		a.launchVersion(instance.Key())
	}
}
