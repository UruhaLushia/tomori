//go:build !windows

package minecraft

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var operatingSystemVersion = sync.OnceValue(func() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "uname", "-r")
	if runtime.GOOS == "darwin" {
		cmd = exec.CommandContext(ctx, "sw_vers", "-productVersion")
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
})
