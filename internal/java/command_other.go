//go:build !windows

package java

import "os/exec"

func prepareCommand(*exec.Cmd) {}
