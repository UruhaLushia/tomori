package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type downloadJob struct {
	data  page.DownloadJob
	index map[string]int
}

func (job *downloadJob) update(status string, progress *download.Progress) {
	if !job.data.Active {
		return
	}
	if !job.data.Canceling {
		job.data.Status = status
	}
	job.data.BytesPerSecond = 0
	if progress == nil {
		return
	}
	job.data.BytesPerSecond = progress.BytesPerSecond
	for _, task := range progress.Tasks {
		if _, exists := job.index[task.Path]; !exists {
			job.index[task.Path] = len(job.data.Files)
			job.data.Total += max(0, task.Size)
			job.data.Files = append(job.data.Files, download.TaskProgress{Task: download.Task{Path: task.Path, Size: task.Size}, State: download.TaskQueued})
		}
	}
	for _, file := range progress.Updates {
		if index, exists := job.index[file.Task.Path]; exists {
			previous := job.data.Files[index]
			if previous.State != file.State {
				job.data.Revision++
			}
			job.data.Total += max(0, file.Task.Size) - max(0, previous.Task.Size)
			job.data.Done += downloadBytes(file) - downloadBytes(previous)
			if previous.State != download.TaskCompleted && file.State == download.TaskCompleted {
				job.data.FilesDone++
			} else if previous.State == download.TaskCompleted && file.State != download.TaskCompleted {
				job.data.FilesDone--
			}
			file.Task = download.Task{Path: file.Task.Path, Size: file.Task.Size}
			job.data.Files[index] = file
		}
	}
}

func downloadBytes(file download.TaskProgress) int64 {
	if file.Task.Size > 0 {
		return max(0, min(file.Done, file.Task.Size))
	}
	return 0
}

func (d *downloadJob) finish(err error) {
	job := &d.data
	job.Canceling = false
	if !job.Active {
		return
	}
	job.Active = false
	job.Revision++
	d.index = nil
	job.BytesPerSecond = 0
	job.Status = locales.Completed()
	if err != nil {
		job.Status = locales.Failed()
		job.Error = err.Error()
		if operationCanceled(err) {
			job.Status, job.Error = locales.Canceled(), ""
		}
		for index := range job.Files {
			file := &job.Files[index]
			if file.State == download.TaskQueued || file.State == download.TaskChecking || file.State == download.TaskDownloading {
				file.State = download.TaskCanceled
			}
		}
	}
}

func (a *app) downloadButton(c *ui.Context) {
	jobs := a.downloadJobs(c)
	done, total := float64(0), float64(0)
	failure, completed := false, false
	for _, op := range a.operations {
		if !op.visible {
			continue
		}
		age := c.Now().Sub(op.finishedAt)
		if op.job.data.Active {
			weight := max(1, float64(op.job.data.Total))
			done += weight * op.job.data.Fraction()
			total += weight
		} else if op.job.data.Error != "" && !op.viewed {
			failure = true
		} else if op.job.data.Error == "" && !op.finishedAt.IsZero() && age < 3*time.Second {
			completed = true
			c.After(3*time.Second - age)
		}
	}
	if total == 0 && !failure && !completed && !a.downloadOpen {
		return
	}
	c.Key("download-tasks")
	trigger := kit.TaskIndicator(c, done/max(1, total), total > 0, failure, func() { a.downloadOpen = !a.downloadOpen }).
		Label(locales.Downloads4()).Tooltip(locales.Downloads4()).Expanded(a.downloadOpen)
	if a.downloadOpen {
		trigger.Background(kit.Press)
	}
	a.pages.downloadTasks.View(c, trigger, &a.downloadOpen, jobs, a.cancelOperation)
}

func (a *app) pruneOperations() {
	completed := len(a.operationOrder) - a.activeOperations
	kept := a.operationOrder[:0]
	for _, id := range a.operationOrder {
		op := a.operations[id]
		if !op.job.data.Active && completed > operationLimit {
			delete(a.operations, id)
			completed--
		} else {
			kept = append(kept, id)
		}
	}
	a.operationOrder = kept
}

func (a *app) discardCompletedOperations() {
	kept := a.operationOrder[:0]
	for _, id := range a.operationOrder {
		if a.operations[id].job.data.Active {
			kept = append(kept, id)
		} else {
			delete(a.operations, id)
		}
	}
	a.operationOrder = kept
	clear(a.downloadRows)
	a.downloadRows = a.downloadRows[:0]
}

func (a *app) downloadJobs(c *ui.Context) []page.DownloadJob {
	if a.downloadOpen {
		a.downloadClosed = time.Time{}
		for _, op := range a.operations {
			if !op.job.data.Active {
				op.viewed = true
			}
		}
	} else {
		if a.downloadClosed.IsZero() {
			a.downloadClosed = c.Now()
		}
		if c.Now().Sub(a.downloadClosed) >= 200*time.Millisecond {
			kept := a.operationOrder[:0]
			for _, id := range a.operationOrder {
				op := a.operations[id]
				finished := !op.finishedAt.IsZero() && c.Now().Sub(op.finishedAt) >= 3*time.Second
				if !op.job.data.Active && (op.job.data.Error != "" && op.viewed || op.job.data.Error == "" && finished) {
					delete(a.operations, id)
				} else {
					kept = append(kept, id)
				}
			}
			a.operationOrder = kept
		} else {
			c.After(200 * time.Millisecond)
		}
	}
	clear(a.downloadRows)
	a.downloadRows = a.downloadRows[:0]
	for _, id := range a.operationOrder {
		if op := a.operations[id]; op.visible {
			a.downloadRows = append(a.downloadRows, op.job.data)
		}
	}
	return a.downloadRows
}
