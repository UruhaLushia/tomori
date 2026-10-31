//go:build !no_gui

package main

import (
	"os"

	"github.com/UruhaLushia/tomori/internal/launchrequest"
)

func instanceRequest(launch bool, selector string) (launchrequest.Request, error) {
	directory, err := os.Getwd()
	if err != nil {
		return launchrequest.Request{}, err
	}
	name := "open"
	if launch {
		name = "launch"
	}
	return launchrequest.Parse([]string{name, selector}, directory)
}
