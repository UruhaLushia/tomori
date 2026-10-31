package config

type LauncherVisibility string
type LauncherClose string

const (
	LauncherKeepVisible  LauncherVisibility = "keep"
	LauncherMinimize     LauncherVisibility = "minimize"
	LauncherHide         LauncherVisibility = "hide"
	LauncherStopGames    LauncherClose      = "stop"
	LauncherWaitForGames LauncherClose      = "wait"
)

type Launcher struct {
	Visibility LauncherVisibility `json:"visibility"`
	Close      LauncherClose      `json:"close"`
}

func (l *Launcher) normalize() {
	switch l.Visibility {
	case LauncherKeepVisible, LauncherMinimize, LauncherHide:
	default:
		l.Visibility = LauncherKeepVisible
	}
	if l.Close != LauncherWaitForGames {
		l.Close = LauncherStopGames
	}
}
