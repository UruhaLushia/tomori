//go:build !linux

package app

import "github.com/egoist/mygo"

func prepareZoomOutline(window, _ *mygo.Window, target mygo.Rectangle) error {
	window.SetIgnoreMouseEvents(true)
	window.SetContentBounds(target)
	return nil
}
