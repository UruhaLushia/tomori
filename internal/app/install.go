package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
)

const operationLimit = 8

type operation struct {
	sourceID, installName, resultKey string
	failureTitle                     string
	cancel                           context.CancelFunc
	key, content                     string
	target                           string
	visible                          bool
	viewed                           bool
	finishedAt                       time.Time
	job                              downloadJob
}

func (a *app) hasTasks() bool  { return a.activeOperations > 0 }
func (a *app) tasksFull() bool { return a.activeOperations >= operationLimit }

func (a *app) operationCurrent(token uint64) bool {
	op := a.operations[token]
	return op != nil && op.job.data.Active
}

func (a *app) beginOperation(label, key, content string) (context.Context, *launcher.Service, uint64) {
	ctx, token := a.beginTask(label, key, content)
	if token == 0 {
		return nil, nil, 0
	}
	a.operations[token].visible = true
	return ctx, a.newService(), token
}

func (a *app) beginTask(label, key, content string) (context.Context, uint64) {
	if a.tasksFull() || a.ctx.Err() != nil {
		return nil, 0
	}
	for _, op := range a.operations {
		if key != "" && op.key == key && op.job.data.Active {
			return nil, 0
		}
	}
	a.pruneOperations()
	a.operationID++
	ctx, cancel := context.WithCancel(a.ctx)
	if a.operations == nil {
		a.operations = make(map[uint64]*operation)
	}
	a.operations[a.operationID] = &operation{cancel: cancel, key: key, content: content, job: downloadJob{
		data: page.DownloadJob{ID: a.operationID, Title: label, Status: locales.Preparing(), Active: true}, index: make(map[string]int),
	}}
	a.operationOrder = append(a.operationOrder, a.operationID)
	a.activeOperations++
	return ctx, a.operationID
}

func (a *app) completeOperation(token uint64, err error, keepContext bool) {
	op := a.operations[token]
	if op == nil || !op.job.data.Active {
		return
	}
	op.job.finish(err)
	op.finishedAt = time.Now()
	if err != nil {
		title := op.failureTitle
		if title == "" {
			title = op.job.data.Title
		}
		a.notifyError(title, err)
	} else if !keepContext {
		if op.resultKey != "" {
			a.notifyInstalled(op.sourceID, op.installName, op.resultKey)
		} else {
			a.notify(locales.Completed(), op.job.data.Title, false)
		}
	}
	a.activeOperations--
	if !keepContext && op.cancel != nil {
		op.cancel()
	}
	op.cancel = nil
	op.key, op.content, op.target = "", "", ""
	a.pruneOperations()
	if a.launcherVisibility != "" {
		a.discardCompletedOperations()
	}
}

func (a *app) finishOperation(token uint64, err error) {
	a.win.Update(func() {
		a.completeOperation(token, err, false)
	})
	if operationCanceled(err) {
		a.logf("%s", locales.OperationCanceled())
	} else if err != nil {
		a.logf(locales.OperationFailed(), err)
	}
}

func (a *app) cancelOperation(token uint64) {
	op := a.operations[token]
	if op == nil || !op.job.data.Active || op.job.data.Canceling {
		return
	}
	for key, session := range a.games {
		if !session.activity.Running && session.token == token {
			a.stopInstance(key)
			return
		}
	}
	if op.cancel != nil {
		op.cancel()
		op.job.data.Status = locales.Canceling()
		op.job.data.Canceling = true
	}
}

func stageLabel(p minecraft.Progress) string {
	var name string
	switch p.Stage {
	case "space":
		name = locales.CheckingDiskSpace()
	case "waiting":
		name = locales.Queued()
	case "client":
		name = locales.DownloadingClient()
	case "libraries":
		name = locales.DownloadingLibraries()
	case "natives":
		name = locales.DownloadingNativeLibraries()
	case "logging":
		name = locales.DownloadingLogConfiguration()
	case "assets":
		name = locales.DownloadingAssets()
	case "done":
		name = locales.FinishingInstallation()
	case "java":
		name = locales.PreparingJava()
	case "download":
		name = locales.Downloading()
	case "prepare":
		name = locales.CheckingFiles()
	case "copy":
		name = locales.CopyingFiles()
	case "world-import":
		name = locales.ImportWorld()
	case "world-backup":
		name = locales.BackUpWorld()
	case "extract":
		name = locales.ExtractingNativeLibraries()
	case "loader":
		name = locales.InstallingLoader()
	case "packfiles":
		name = locales.DownloadingModpackFiles()
	case "pack-update":
		name = locales.UpdatingPackFiles()
	case "pack-update-config":
		name = locales.SavingPackSettings()
	case "pack-extract":
		name = locales.ExtractingModpack()
	case "pack-compare":
		name = locales.ComparingPackFiles()
	case "pack-restore-check":
		name = locales.CheckingBackup()
	case "pack-restore":
		name = locales.RestoreModpack()
	case "metadata":
		name = locales.ReadingLaunchConfiguration()
	case "java-check":
		name = locales.ChooseJava()
	case "launch":
		name = locales.LaunchGame()
	case "pre-launch-command":
		name = locales.PreLaunchCommand()
	case "post-exit-command":
		name = locales.PostExitCommand()
	default:
		name = locales.PreparingInstallation()
	}
	if p.Download == nil && p.Files > 1 {
		return fmt.Sprintf("%s %d/%d", name, p.FilesDone, p.Files)
	}
	return name
}

func (a *app) progressHandler(token uint64) func(minecraft.Progress) {
	var mu sync.Mutex
	var last time.Time
	var stage string
	return func(p minecraft.Progress) {
		mu.Lock()
		defer mu.Unlock()
		if p.Download == nil && stage == p.Stage && time.Since(last) < 100*time.Millisecond && (p.Total <= 0 || p.Done < p.Total) {
			return
		}
		stage, last = p.Stage, time.Now()
		label := stageLabel(p)
		a.win.Update(func() {
			if p.Stage == "post-exit-command" {
				for _, session := range a.games {
					if session.token == token && !session.activity.Canceling {
						session.activity.Finishing = true
						session.activity.Status = label
					}
				}
			}
			if a.operationCurrent(token) {
				if p.Space != nil {
					a.operations[token].job.data.Space = p.Space
				}
				for _, session := range a.games {
					if session.token == token && !session.activity.Running && !session.activity.Canceling {
						session.activity.Status = label
					}
				}
				a.operations[token].job.update(label, p.Download)
			}
		})
	}
}

func (a *app) install(id, dir string, kind modpack.Loader, name string) {
	if a.tasksFull() || a.pathBusy {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 128 || strings.ContainsFunc(name, unicode.IsControl) {
		a.notifyError(locales.InstallationFailed(), fmt.Errorf("%s", locales.Use1128CharactersWithoutControlCharacters()))
		return
	}
	summary, ok := a.summaryByID(id)
	if !ok {
		a.logf(locales.NotFoundInVersionManifest(), id)
		return
	}
	if dir == "" {
		dir = config.DefaultGameDirectory()
	}
	allowed := false
	for _, entry := range a.installDirectories() {
		if entry.Path == dir {
			allowed = true
			break
		}
	}
	if !allowed {
		a.logf("%s", locales.InstallLocationUnavailableChooseAnother())
		return
	}
	for _, instance := range a.installedList {
		if instance.ID == id && instance.GameDir == dir && a.instanceInUse(instance.Key()) {
			return
		}
	}
	found := false
	for _, entry := range a.cfg.Directories.Entries {
		found = found || entry.Path == dir
	}
	if !found {
		if err := a.cfg.Directories.Add(dir, "Tomori"); err != nil {
			a.logf(locales.FailedToAddInstallLocation(), err)
			return
		}
		a.saveConfig()
	}
	key, content := "install:"+dir+":"+id+":"+kind.String(), ""
	if kind.ID == "" {
		key = (minecraft.Instance{Format: minecraft.InstanceMinecraft, InstanceDir: filepath.Join(dir, "versions", id)}).Key()
		content = filepath.Join(dir, "versions", id)
	}
	ctx, service, token := a.beginOperation(locales.GameClient()+id, key, content)
	if token == 0 {
		return
	}
	a.operations[token].sourceID, a.operations[token].installName = "game:"+id, name
	a.operations[token].failureTitle = locales.InstallationFailed()
	selected := a.currentVersion()
	progress := a.progressHandler(token)
	a.logf(locales.Installing(), id, dir)
	a.workers.Go(func() {
		defer service.Close()
		instance, err := service.InstallWithLoader(ctx, dir, summary, kind, progress, func(line string) { a.logf("%s", line) })
		if err == nil {
			if name != instance.Name {
				err = minecraft.RenameInstance(ctx, instance, name)
			}
			if err == nil {
				a.logf(locales.Installed2(), id)
			}
			a.win.Update(func() {
				if !a.operationCurrent(token) || a.ctx.Err() != nil {
					return
				}
				for _, entry := range a.cfg.Directories.Entries {
					if entry.Path == dir {
						if a.currentVersion() == selected {
							a.cfg.Directories.SelectedInstance = instance.Key()
						}
						a.saveConfig()
						break
					}
				}
				a.scanInstalled()
				a.operations[token].resultKey = instance.Key()
				a.pages.instances.Reveal(instance.Key())
			})
		}
		a.finishOperation(token, err)
	})
}

func (a *app) repair(id string) {
	if a.tasksFull() || a.pathBusy || a.instanceInUse(id) {
		return
	}
	if a.offerPackRecovery(id) {
		return
	}
	instance, ok := a.instanceByKey(id)
	if !ok || !instance.CanLaunch() {
		return
	}
	ctx, service, token := a.beginOperation(locales.VerifyAndRepair2()+instance.Name, id, instance.ContentDirectory(a.cfg.GameForInstance(id).Isolate))
	if token == 0 {
		return
	}
	progress := a.progressHandler(token)
	a.workers.Go(func() {
		defer service.Close()
		err := service.Repair(ctx, instance, progress)
		if err == nil {
			a.logf(locales.VerifiedAndRepaired(), instance.Name)
		}
		a.finishOperation(token, err)
	})
}

func (a *app) installDirectories() []config.GameDirectory {
	path := config.DefaultGameDirectory()
	entries := []config.GameDirectory{{Name: "Tomori", Path: path}}
	for _, entry := range a.cfg.Directories.Entries {
		if entry.Path == path {
			entries[0] = entry
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}
