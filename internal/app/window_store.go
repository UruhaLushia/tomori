package app

import (
	"github.com/egoist/mygo"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/locales"
)

// MyGo handles displays and restoration; our store keeps all state in one file.
type windowStateStore struct{ app *app }

func (s windowStateStore) LoadWindowStates() (map[string]mygo.WindowState, error) {
	w := s.app.cfg.Window
	if w.Width <= 0 || w.Height <= 0 {
		return nil, nil
	}
	return map[string]mygo.WindowState{"main": {
		X: w.X, Y: w.Y, Width: w.Width, Height: w.Height, Maximized: w.Maximized, FullScreen: w.FullScreen,
	}}, nil
}

func (s windowStateStore) SaveWindowStates(states map[string]mygo.WindowState) error {
	if w, ok := states["main"]; ok {
		s.app.cfg.Window = config.Window{X: w.X, Y: w.Y, Width: w.Width, Height: w.Height, Maximized: w.Maximized, FullScreen: w.FullScreen}
	}
	err := s.app.persistConfig()
	if err != nil {
		s.app.logf(locales.FailedToSaveWindowState(), err)
	}
	return err
}
