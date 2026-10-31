package launcher

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/locales"
)

type CommandError struct {
	Phase        func() string
	Err          error
	GameExitCode int
}

func (e *CommandError) Error() string {
	return fmt.Sprintf(locales.CommandFailed(), e.Phase(), e.Err)
}
func (e *CommandError) Unwrap() error { return e.Err }

func (c LaunchConfig) commandEnvironment() []string {
	return []string{
		"TOMORI_GAME_DIR=" + c.GameDir,
		"TOMORI_INSTANCE_DIR=" + c.InstanceDir,
		"TOMORI_INSTANCE_NAME=" + c.InstanceName,
		"TOMORI_JAVA_PATH=" + c.JavaPath,
		"TOMORI_VERSION=" + c.Version.ID,
	}
}

func (c LaunchConfig) runHook(ctx context.Context, phase func() string, postExit bool, command string, env []string, code int, output io.Writer) error {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.Execution.Timeout())*time.Second)
	defer cancel()
	cmd := hookCommand(ctx, command)
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = c.GameDir, env, output, output
	if postExit {
		cmd.Env = append(append([]string(nil), env...), "TOMORI_EXIT_CODE="+strconv.Itoa(code))
	}
	cmd.WaitDelay = 3 * time.Second
	err := cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		return &CommandError{Phase: phase, Err: err, GameExitCode: code}
	}
	return nil
}
