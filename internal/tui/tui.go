//go:build !no_tui

package tui

import "errors"

var ErrNotImplemented = errors.New("TUI is not implemented yet")

func Run() error { return ErrNotImplemented }
