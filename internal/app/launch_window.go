package app

import (
	"math"
	"os"
	"runtime"

	"github.com/egoist/mygo"
)

func xwaylandDisplaySizes() [][2]int {
	if runtime.GOOS != "linux" || os.Getenv("WAYLAND_DISPLAY") == "" || os.Getenv("DISPLAY") == "" {
		return nil
	}
	var sizes [][2]int
	for _, display := range mygo.Screen.Displays() {
		bounds := display.Bounds
		if bounds.Width <= 0 || bounds.Height <= 0 {
			continue
		}
		sizes = append(sizes, [2]int{bounds.Width, bounds.Height})
		if display.ScaleFactor > 0 && display.ScaleFactor != 1 {
			// Cover both logical coordinates and XWayland's pixel coordinates.
			sizes = append(sizes, [2]int{int(math.Round(float64(bounds.Width) * display.ScaleFactor)), int(math.Round(float64(bounds.Height) * display.ScaleFactor))})
		}
	}
	return sizes
}
