package page

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type DownloadJob struct {
	ID                   uint64
	Title, Status, Error string
	Active, Canceling    bool
	Done, Total          int64
	BytesPerSecond       float64
	FilesDone            int
	Files                []download.TaskProgress
	Revision             uint64
	Space                *storage.Report
}

func (d DownloadJob) Fraction() float64 {
	if d.Total > 0 {
		return min(1, float64(d.Done)/float64(d.Total))
	}
	if len(d.Files) > 0 {
		return float64(d.FilesDone) / float64(len(d.Files))
	}
	return 0
}

type DownloadTasks struct {
	tasks map[uint64]*downloadTaskView
}

type downloadTaskView struct {
	expanded bool
	showAll  bool
	list     ui.ListState
	visible  []int
	source   *download.TaskProgress
	count    int
	revision uint64
}

func (p *DownloadTasks) View(c *ui.Context, trigger ui.Element, open *bool, jobs []DownloadJob, cancel func(uint64)) {
	for id := range p.tasks {
		found := false
		for _, job := range jobs {
			if job.ID == id {
				found = true
				break
			}
		}
		if !found {
			delete(p.tasks, id)
		}
	}
	ui.PopoverBase(c, trigger, open, func(panel ui.Element) {
		w, h := c.Size()
		anchor := trigger.Bounds()
		kit.PopupPanel(c, panel, kit.W(320), trigger).MinWidth(0).MaxWidth(max(0, w-16)).
			MaxHeight(max(0, min(360, h-anchor.Y-anchor.H-12))).
			AttachTo(trigger, ui.AnchorBottomEnd, ui.AnchorTopEnd).Label(locales.Downloads4())
		panel.Children(func() {
			ui.Row(c).FillWidth().MinHeight(kit.ControlH()).Padding(0, 8).AlignItems(ui.Center).Shrink(0).Children(func() {
				kit.Line(c, locales.Downloads4(), kit.Strong).Grow(1)
			})
			if len(jobs) == 0 {
				kit.Label(c, locales.NoDownloads(), kit.Body).TextColor(kit.Muted).Padding(8)
				return
			}
			ui.Scroll(c).Scrollbars(false).MinHeight(0).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					for i, job := range jobs {
						if i > 0 {
							kit.Divider(c).Margin(4, 8)
						}
						if p.tasks == nil {
							p.tasks = make(map[uint64]*downloadTaskView)
						}
						task := p.tasks[job.ID]
						if task == nil {
							task = &downloadTaskView{}
							p.tasks[job.ID] = task
						}
						c.Key(job.ID)
						task.row(c, job, cancel)
					}
				})
			})
		})
	})
}

func (p *downloadTaskView) row(c *ui.Context, job DownloadJob, cancel func(uint64)) {
	kit.PresenceMotion(kit.LayoutMotion(ui.Column(c).FillWidth().Padding(4, 8).Gap(4).Radius(kit.RadiusM))).Children(func() {
		kit.RowButton(c, "", false, func() { p.expanded = !p.expanded }).FillWidth().MinHeight(kit.TabH()).Gap(8).Label(job.Title).Expanded(p.expanded).Children(func() {
			kit.Line(c, job.Title, kit.Strong).Grow(1).Tooltip(job.Title)
			rotation := float32(0)
			if p.expanded {
				rotation = 90
			}
			ui.Icon(c, kit.ChevronRight).FontSize(12).AnimatePose(ui.Pose{Rotate: rotation}, kit.MotionMove).TextColor(kit.Muted)
		})
		size := fmt.Sprintf("%s / %s", transferSize(job.Done), transferSize(job.Total))
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			status := job.Status
			if len(job.Files) > 0 {
				status += fmt.Sprintf(" · %d / %d", job.FilesDone, len(job.Files))
			}
			color := kit.Muted
			if job.Error != "" {
				color = kit.Red
			}
			kit.Line(c, status, kit.Caption).Grow(1).TextColor(color).Tooltip(status).ContentTransition(kit.ValueMotion)
			if !job.Active && job.Total > 0 {
				kit.Line(c, size, kit.Caption).Tooltip(size)
			}
		})
		if job.Active {
			c.Key("task-progress")
			kit.FoldMotion(kit.Progress(c, job.Fraction()).FillWidth().Height(4))
		}
		if p.expanded {
			kit.FoldMotion(ui.Column(c.Key("task-details")).FillWidth().Gap(8)).Children(func() {
				if job.Space != nil {
					SpaceSummary(c, SpaceData{Report: *job.Space})
				}
				p.fileList(c, job)
				if job.Error != "" {
					kit.ErrorText(c, job.Error)
				}
			})
		}
		if job.Active {
			kit.LayoutMotion(ui.Row(c.Key("task-footer")).FillWidth().Gap(8).AlignItems(ui.Center)).Children(func() {
				kit.Line(c, transferSize(int64(job.BytesPerSecond))+"/s", kit.Caption).Grow(1)
				if job.Total > 0 {
					kit.Line(c, size, kit.Caption).Tooltip(size)
				}
				label := locales.CancelDownload()
				if job.Canceling {
					label = locales.Canceling()
				}
				kit.IconButton(c, kit.Close, false, func() { cancel(job.ID) }).Size(28, 28).
					Label(job.Title + " · " + label).Tooltip(label).Disabled(job.Canceling)
			})
		}
	})
}

func (p *downloadTaskView) preview(job DownloadJob) {
	files := job.Files
	var source *download.TaskProgress
	if len(files) > 0 {
		source = &files[0]
	}
	if source == p.source && len(files) == p.count && job.Revision == p.revision {
		return
	}
	p.source, p.count, p.revision = source, len(files), job.Revision
	p.visible = p.visible[:0]
	for _, state := range []download.TaskState{download.TaskDownloading, download.TaskChecking, download.TaskFailed, download.TaskQueued, download.TaskCanceled, download.TaskCompleted} {
		for index, file := range files {
			if file.State == state {
				p.visible = append(p.visible, index)
				if len(p.visible) == 5 {
					return
				}
			}
		}
	}
}

func (p *downloadTaskView) fileList(c *ui.Context, job DownloadJob) {
	files := job.Files
	if p.showAll && len(files) > 5 {
		c.Key("all-download-files")
		kit.PresenceMotion(ui.List(c, &p.list, len(files)).Height(320).FillWidth().Padding(0, 12, 0, 0)).
			ItemKey(func(index int) any { return files[index].Task.Path }).Scrollbars(false).Rows(func(row ui.ListRow) {
			downloadFile(row.Context, files[row.Index])
		})
	} else if len(files) > 0 {
		p.preview(job)
		c.Key("download-files-preview")
		kit.PresenceMotion(ui.Column(c).FillWidth()).Children(func() {
			for _, index := range p.visible {
				c.Key(files[index].Task.Path)
				downloadFile(c, files[index])
			}
		})
	}
	if len(files) > 5 {
		label := fmt.Sprintf(locales.ShowAll(), len(files)-5)
		if p.showAll {
			label = locales.Collapse()
		}
		c.Key("toggle-download-files")
		kit.Ghost(c, label, func() { p.showAll = !p.showAll; p.list = ui.ListState{} }).TextColor(kit.AccentText)
	}
}

func downloadFile(c *ui.Context, file download.TaskProgress) {
	kit.LayoutMotion(ui.Column(c).FillWidth().Gap(4).Padding(8, 0)).Children(func() {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			kit.Label(c, filepath.Base(file.Task.Path), kit.Body).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
			kit.Label(c, fileStatus(file), kit.Body).TextColor(kit.Muted)
		})
		// Reserve the progress track's height while a file changes state.
		ui.Box(c).FillWidth().Height(2).Children(func() {
			if file.State == download.TaskDownloading && file.Task.Size > 0 {
				kit.Progress(c, min(1, float64(file.Done)/float64(file.Task.Size))).FillWidth().Height(2)
			}
		})
		kit.Divider(c)
	})
}

func fileStatus(file download.TaskProgress) string {
	switch file.State {
	case download.TaskChecking:
		return locales.Verifying()
	case download.TaskDownloading:
		if file.Task.Size > 0 {
			return fmt.Sprintf("%.0f%%", min(100, float64(file.Done)*100/float64(file.Task.Size)))
		}
		return transferSize(file.Done)
	case download.TaskCompleted:
		return locales.Completed()
	case download.TaskFailed:
		return locales.Failed()
	case download.TaskCanceled:
		return locales.Canceled()
	default:
		return locales.Queued()
	}
}

func transferSize(value int64) string {
	if value >= 1<<30 {
		return fmt.Sprintf("%.2f GiB", float64(value)/(1<<30))
	}
	if value >= 1<<20 {
		return fmt.Sprintf("%.1f MiB", float64(value)/(1<<20))
	}
	return fmt.Sprintf("%.0f KiB", float64(value)/(1<<10))
}
