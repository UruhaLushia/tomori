package launcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/logstore"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

type Process struct {
	cmd            *exec.Cmd
	log            *logstore.File
	stdout, stderr *lineWriter
	once           sync.Once
	err            error
	after          func() error
	LogPath        string
}

func (p *Process) PID() int { return p.cmd.Process.Pid }

// exec.Cmd owns output copying; Wait drains both streams before closing the log.
func (p *Process) Wait() error {
	p.once.Do(func() {
		p.err = p.cmd.Wait()
		p.stdout.finish()
		p.stderr.finish()
		if p.after != nil {
			err := p.after()
			p.after = nil
			if err != nil {
				p.err = errors.Join(p.err, err)
			}
		}
		if p.err == nil {
			p.err = p.stdout.err
		}
		if p.err == nil {
			p.err = p.stderr.err
		}
		if err := p.log.Close(); p.err == nil {
			p.err = err
		}
	})
	return p.err
}

func Run(ctx context.Context, cfg LaunchConfig, logPath string, onLine func(string), stores ...*logstore.Store) (_ *Process, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := cfg.Execution.Validate(); err != nil {
		return nil, err
	}
	if cfg.ProcessPriority == "" {
		cfg.ProcessPriority = config.PriorityNormal
	}
	if !cfg.ProcessPriority.Valid() {
		return nil, fmt.Errorf(locales.InvalidProcessPriorityAlt(), cfg.ProcessPriority)
	}
	args, err := cfg.Command()
	if err != nil {
		return nil, err
	}
	var logs *logstore.Store
	if len(stores) > 0 {
		logs = stores[0]
	}
	if logs == nil {
		logs, err = logstore.New(filepath.Dir(logPath), 256<<20)
		if err != nil {
			return nil, err
		}
	}
	log, err := logs.Open(filepath.Base(logPath), false)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = log.Close()
		}
	}()
	var mu sync.Mutex
	var secrets []string
	var redactor *strings.Replacer
	if cfg.Execution.Enabled {
		secrets = cfg.Execution.Secrets()
		slices.SortFunc(secrets, func(a, b string) int { return len(b) - len(a) })
		if len(secrets) > 0 {
			pairs := make([]string, 0, len(secrets)*2)
			for _, secret := range secrets {
				pairs = append(pairs, secret, "[redacted]")
			}
			redactor = strings.NewReplacer(pairs...)
		}
	}
	stdout, stderr := &logOutput{emit: onLine}, &logOutput{emit: onLine}
	sink := func(output *logOutput) func(string) error {
		return func(line string) error {
			mu.Lock()
			defer mu.Unlock()
			line = cfg.Account.Redact(line)
			for _, secret := range secrets {
				if strings.Contains(line, secret) {
					line = redactor.Replace(line)
					break
				}
			}
			if _, err := io.WriteString(log, line+"\n"); err != nil {
				return err
			}
			output.write(line)
			return nil
		}
	}
	emit := sink(stdout)
	p := &Process{cmd: exec.CommandContext(ctx, cfg.JavaPath, args...), log: log, LogPath: logPath}
	p.stdout = &lineWriter{emit: emit, done: stdout.flush}
	p.stderr = &lineWriter{emit: sink(stderr), done: stderr.flush}
	p.cmd.Dir, p.cmd.Stdout, p.cmd.Stderr = cfg.GameDir, p.stdout, p.stderr
	p.cmd.WaitDelay = 3 * time.Second
	if cfg.Execution.Enabled {
		variables, err := cfg.Execution.Variables()
		if err != nil {
			return nil, err
		}
		p.cmd.Env = append(p.cmd.Environ(), variables...)
		p.cmd.Env = append(p.cmd.Env, cfg.commandEnvironment()...)
		output := &logOutput{emit: onLine}
		writer := &lineWriter{emit: sink(output), done: output.flush}
		if strings.TrimSpace(cfg.Execution.Before) != "" {
			if cfg.Progress != nil {
				cfg.Progress(minecraft.Progress{Stage: "pre-launch-command"})
			}
			err := cfg.runHook(ctx, locales.PreLaunchCommand, false, cfg.Execution.Before, p.cmd.Env, 0, writer)
			writer.finish()
			if err == nil {
				err = writer.err
			}
			if err != nil {
				return nil, err
			}
		}
		if strings.TrimSpace(cfg.Execution.After) != "" {
			p.after = func() error {
				if ctx.Err() != nil {
					return nil
				}
				if cfg.Progress != nil {
					cfg.Progress(minecraft.Progress{Stage: "post-exit-command"})
				}
				err := cfg.runHook(ctx, locales.PostExitCommand, true, cfg.Execution.After, p.cmd.Env, p.cmd.ProcessState.ExitCode(), writer)
				writer.finish()
				if err == nil {
					err = writer.err
				}
				return err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := setGameFullscreen(cfg.GameDir, cfg.FullScreen); err != nil {
		return nil, fmt.Errorf(locales.FullscreenSettingSyncFailed(), err)
	}
	mode := locales.Window()
	if cfg.FullScreen {
		mode = locales.Fullscreen()
	}
	if err := emit(fmt.Sprintf("%s · %d × %d", mode, cfg.Width, cfg.Height)); err != nil {
		return nil, err
	}
	if err := p.cmd.Start(); err != nil {
		return nil, err
	}
	if err := setProcessPriority(p.PID(), cfg.ProcessPriority); err != nil {
		_ = emit(fmt.Sprintf(locales.ProcessPrioritySetFailed(), cfg.ProcessPriority, err))
	}
	return p, nil
}

type lineWriter struct {
	buf  []byte
	emit func(string) error
	done func()
	err  error
}

func (w *lineWriter) Write(data []byte) (int, error) {
	size := len(data)
	if w.err != nil {
		return 0, w.err
	}
	for len(data) > 0 {
		n := bytes.IndexByte(data, '\n')
		if n < 0 {
			n = len(data)
		}
		// Bound memory for a process that writes a very long line.
		chunk := min(n, 1<<20-len(w.buf))
		w.buf = append(w.buf, data[:chunk]...)
		data = data[chunk:]
		if chunk == n && len(data) > 0 && data[0] == '\n' {
			data = data[1:]
			w.flush()
		} else if len(w.buf) == 1<<20 {
			w.flush()
		}
		if w.err != nil {
			return 0, w.err
		}
	}
	return size, nil
}

func (w *lineWriter) flush() {
	if len(w.buf) > 0 && w.err == nil {
		w.err = w.emit(string(bytes.TrimSuffix(w.buf, []byte{'\r'})))
	}
	w.buf = w.buf[:0]
}

func (w *lineWriter) finish() {
	w.flush()
	if w.done != nil {
		w.done()
	}
}
