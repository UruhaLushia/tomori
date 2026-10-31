package app

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo"
)

type outlineNative struct {
	ignore func(uintptr)
	place  func(uintptr, uintptr)
}

// MyGo's Linux SetIgnoreMouseEvents is empty, and flyouts normally fit the screen.
var zoomOutlineNative = sync.OnceValues(func() (outlineNative, error) {
	gtk, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return outlineNative{}, err
	}
	gdk, err := purego.Dlopen("libgdk-3.so.0", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return outlineNative{}, err
	}
	cairo, err := purego.Dlopen("libcairo.so.2", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return outlineNative{}, err
	}
	type rectangle struct{ X, Y, Width, Height int32 }
	var region func() uintptr
	var destroy func(uintptr)
	var inputShape func(uintptr, uintptr)
	var nativeWindow func(uintptr) uintptr
	var child func(uintptr) uintptr
	var allocation func(uintptr, *rectangle)
	var moveToRect func(uintptr, *rectangle, int32, int32, int32, int32, int32)
	purego.RegisterLibFunc(&region, cairo, "cairo_region_create")
	purego.RegisterLibFunc(&destroy, cairo, "cairo_region_destroy")
	purego.RegisterLibFunc(&inputShape, gtk, "gtk_widget_input_shape_combine_region")
	purego.RegisterLibFunc(&nativeWindow, gtk, "gtk_widget_get_window")
	purego.RegisterLibFunc(&child, gtk, "gtk_bin_get_child")
	purego.RegisterLibFunc(&allocation, gtk, "gtk_widget_get_allocation")
	purego.RegisterLibFunc(&moveToRect, gdk, "gdk_window_move_to_rect")
	return outlineNative{
		ignore: func(widget uintptr) {
			r := region()
			inputShape(widget, r)
			destroy(r)
		},
		place: func(widget, parent uintptr) {
			var r rectangle
			allocation(child(parent), &r)
			// GDK_GRAVITY_CENTER, with no flip, slide or resize constraints.
			moveToRect(nativeWindow(widget), &r, 5, 5, 0, 0, 0)
		},
	}, nil
})

func prepareZoomOutline(window, parent *mygo.Window, _ mygo.Rectangle) error {
	native, err := zoomOutlineNative()
	if err != nil {
		return err
	}
	native.place(window.NativeHandle(), parent.NativeHandle())
	apply := func() {
		if widget := window.NativeHandle(); widget != 0 {
			native.ignore(widget)
		}
	}
	window.OnShow(func() { window.Update(apply) })
	window.OnResize(func() { window.Update(apply) })
	return nil
}
