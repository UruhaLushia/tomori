//go:build !unix && !windows

package launcher

import (
	"context"
	"os/exec"
)

func hookCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", command)
}
