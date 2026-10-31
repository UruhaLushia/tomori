package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/network"
	"github.com/UruhaLushia/tomori/locales"
)

type gameSession struct {
	token    uint64
	cancel   context.CancelFunc
	activity page.InstanceActivity
	content  string
	pid      int
	logPath  string
	name     string
	playedAt time.Time
	server   string
}

func (a *app) instanceActivity(key string) page.InstanceActivity {
	if session := a.games[key]; session != nil {
		return session.activity
	}
	return page.InstanceActivity{}
}

func (a *app) instanceActive(key string) bool { return a.games[key] != nil }

func (a *app) instanceInUse(key string) bool {
	if len(a.games) == 0 && !a.hasTasks() {
		return false
	}
	if a.instanceActive(key) {
		return true
	}
	instance, ok := a.instanceByKey(key)
	if !ok {
		return false
	}
	content := instance.ContentDirectory(a.cfg.GameForInstance(key).Isolate)
	return a.instancePathsInUse(key, instance.InstanceDir, content)
}

func (a *app) instancePathsInUse(key, directory, content string) bool {
	for other, session := range a.games {
		if other == key || pathsOverlap(content, session.content) || pathsOverlap(directory, session.content) {
			return true
		}
	}
	for _, op := range a.operations {
		if op.job.data.Active && (op.key == key || pathsOverlap(content, op.content) || pathsOverlap(directory, op.content) || pathsOverlap(content, op.target) || pathsOverlap(directory, op.target)) {
			return true
		}
	}
	return false
}

func pathsOverlap(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (a *app) stopInstance(key string) {
	if session := a.games[key]; session != nil && !session.activity.Canceling {
		session.activity.Canceling = true
		session.activity.Status = locales.Stopping()
		session.cancel()
		if !session.activity.Running && a.operationCurrent(session.token) {
			op := a.operations[session.token]
			op.job.data.Status = session.activity.Status
			op.job.data.Canceling = true
		}
	}
}

func (a *app) launch() { a.launchVersion(a.currentVersion()) }

func (a *app) launchVersion(id string) {
	a.launchTarget(id, launcher.ServerTarget{})
}

func (a *app) launchTarget(id string, target launcher.ServerTarget) {
	if a.tasksFull() || a.pathBusy || a.instanceInUse(id) {
		return
	}
	if a.offerPackRecovery(id) {
		return
	}
	if a.authBusy {
		return
	}
	identity, ok := a.cfg.SelectedAccount()
	if !ok {
		a.dialogs.pendingLaunch = &pendingLaunch{key: id, target: target}
		a.dialogs.accounts.ShowAdd()
		return
	}
	instance, found := a.instanceByKey(id)
	if !found || !instance.CanLaunch() {
		a.home.failureKey, a.home.failure, a.home.failureAction = id, locales.InstanceUnavailableCheckItsFilesOrReinstall(), "repair"
		a.logf("%s", locales.SelectAnInstalledInstanceFirst())
		return
	}
	ctx, token := a.beginTask(locales.PreparingToLaunch()+instance.Name, id, instance.ContentDirectory(a.cfg.GameForInstance(id).Isolate))
	if token == 0 {
		return
	}
	service := a.newService()
	a.operations[token].failureTitle = locales.LaunchFailed()
	a.home.failureKey, a.home.failure, a.home.failureLog = "", "", ""
	a.home.failureAction = ""
	service.SetWindowedDisplaySizes(xwaylandDisplaySizes())
	if a.games == nil {
		a.games = make(map[string]*gameSession)
	}
	session := &gameSession{token: token, cancel: a.operations[token].cancel, name: instance.Name, server: target.Address, content: instance.ContentDirectory(a.cfg.GameForInstance(id).Isolate), activity: page.InstanceActivity{Active: true, Status: a.operations[token].job.data.Status}}
	a.games[id] = session
	list := slices.Clone(a.java)
	started := time.Now()
	a.logf(locales.Launch2(), instance.Name)
	progress := a.progressHandler(token)
	proxy := a.cfg.Download.Proxy
	budget, accountPath := a.downloadBudget, filepath.Join(a.store.Dir(), fmt.Sprintf("account-%x", sha256.Sum256([]byte(identity.ID))))
	a.workers.Go(func() {
		updated, err := a.ensureAccount(ctx, budget, accountPath, proxy, identity)
		if err != nil {
			service.Close()
			if ctx.Err() != nil {
				a.finishGame(id, session, ctx.Err())
				return
			}
			a.win.Update(func() {
				if a.ctx.Err() == nil && a.games[id] == session && !session.activity.Canceling {
					a.accountError = err.Error()
					a.home.failureAction = "login"
				}
			})
			a.finishGame(id, session, fmt.Errorf(locales.AccountAuthenticationFailed(), account.Diagnostic(err)))
			return
		}
		var targets []launcher.ServerTarget
		if target.Address != "" {
			targets = append(targets, target)
		}
		process, j, err := service.Launch(ctx, instance, updated, list, progress, func(line string) { a.logSource(instance.Name, line) }, targets...)
		service.Close()
		if err != nil {
			a.finishGame(id, session, err)
			return
		}
		a.logf(locales.StartedPidS(), instance.Name, process.PID(), j.Label(), time.Since(started).Seconds())
		a.logf(locales.GameLog(), process.LogPath)
		a.win.Update(func() {
			if a.games[id] == session && a.operationCurrent(token) && a.ctx.Err() == nil {
				a.completeOperation(token, nil, true)
				session.activity.Running = true
				session.pid, session.logPath = process.PID(), process.LogPath
				a.recordPlayStarted(id, session)
				if !session.activity.Canceling {
					session.activity.Status = locales.Running() + instance.Name
					a.gameStarted()
				}
			}
		})
		err = process.Wait()
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if err == nil {
			a.logSource(instance.Name, locales.GameExited())
		}
		a.finishGame(id, session, err)
	})
}

func (a *app) finishGame(key string, session *gameSession, err error) {
	a.win.Update(func() {
		if a.games[key] != session {
			return
		}
		a.recordPlayFinished(key, session)
		if err != nil && !operationCanceled(err) {
			a.home.failureKey, a.home.failure, a.home.failureLog = key, failureReason(err), ""
			if session.logPath != "" {
				a.home.failureLog = filepath.Base(session.logPath)
			}
		}
		delete(a.games, key)
		session.cancel()
		if !session.activity.Running {
			a.completeOperation(session.token, err, false)
		} else {
			title := locales.GameExitedUnexpectedly()
			var command *launcher.CommandError
			if errors.As(err, &command) && command.GameExitCode == 0 {
				title = command.Phase()
			}
			a.notifyError(title, err)
		}
		a.gameEnded()
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		a.logSource(session.name, fmt.Sprintf(locales.OperationFailed(), err))
	}
}

func (a *app) preparingGames() bool {
	for _, session := range a.games {
		if !session.activity.Running {
			return true
		}
	}
	return false
}

func (a *app) ensureAccount(ctx context.Context, budget *download.Budget, path string, proxy network.Proxy, identity account.Account) (account.Account, error) {
	release, err := budget.Lock(ctx, "account:"+identity.ID, path)
	if err != nil {
		return identity, err
	}
	defer release()
	current := make(chan account.Account, 1)
	a.win.Update(func() {
		if a.applyAccountUpdates() {
			a.saveConfig()
		}
		for _, entry := range a.cfg.Accounts.Entries {
			if entry.ID == identity.ID {
				current <- entry
				return
			}
		}
		current <- identity
	})
	select {
	case identity = <-current:
	case <-ctx.Done():
		return identity, ctx.Err()
	}
	client := account.NewClient(proxy, budget)
	defer client.Close()
	updated, err := client.Ensure(ctx, identity)
	if updated != identity {
		a.queueAccountUpdate(updated, identity.SessionToken(), false)
	}
	return updated, err
}
