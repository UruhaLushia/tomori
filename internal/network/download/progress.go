package download

import (
	"sync"
	"time"
)

type TaskState string

const (
	TaskQueued      TaskState = "queued"
	TaskChecking    TaskState = "checking"
	TaskDownloading TaskState = "downloading"
	TaskCompleted   TaskState = "completed"
	TaskFailed      TaskState = "failed"
	TaskCanceled    TaskState = "canceled"
)

type TaskProgress struct {
	Task  Task
	Done  int64
	State TaskState
	Error string
}

type Progress struct {
	Done, Total    int64
	BytesPerSecond float64
	FilesDone      int
	Files          int
	Tasks          []Task
	Updates        []TaskProgress
	Finished       bool
}

type progressReporter struct {
	mu       sync.Mutex
	files    []TaskProgress
	dirty    []int
	marked   []bool
	current  Progress
	received func() int64
	samples  []speedSample
	notify   func(Progress)
}

type speedSample struct {
	at    time.Time
	bytes int64
}

func newProgress(tasks []Task, notify func(Progress), received func() int64) *progressReporter {
	if notify == nil {
		return nil
	}
	p := &progressReporter{files: make([]TaskProgress, len(tasks)), marked: make([]bool, len(tasks)), notify: notify, received: received}
	p.samples = []speedSample{{at: time.Now(), bytes: received()}}
	p.current = Progress{Total: TotalSize(tasks), Files: len(tasks)}
	for index, task := range tasks {
		p.files[index] = TaskProgress{Task: task, State: TaskQueued}
	}
	initial := p.current
	initial.Tasks = tasks
	notify(initial)
	return p
}

// Publish on a timer so a stalled connection also reports its falling speed.
func (p *progressReporter) start() func() {
	if p == nil {
		return func() {}
	}
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				p.mu.Lock()
				p.publish()
				p.mu.Unlock()
			}
		}
	}()
	return func() { close(stop); <-stopped }
}

func (p *progressReporter) update(index int, done int64, state TaskState, message string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	file := &p.files[index]
	done = max(file.Done, done)
	if file.Task.Size > 0 {
		done = min(done, file.Task.Size)
		p.current.Done += done - file.Done
	}
	if state == TaskCompleted && file.State != TaskCompleted {
		p.current.FilesDone++
	}
	file.Done, file.State, file.Error = done, state, message
	p.mark(index)
}

func (p *progressReporter) mark(index int) {
	if !p.marked[index] {
		p.marked[index] = true
		p.dirty = append(p.dirty, index)
	}
}

func (p *progressReporter) finish(err error) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		for index := range p.files {
			file := &p.files[index]
			if file.State != TaskCompleted && file.State != TaskFailed && file.State != TaskCanceled {
				file.State = TaskCanceled
				p.mark(index)
			}
		}
	}
	p.current.Finished = true
	p.publish()
}

func (p *progressReporter) publish() {
	now := time.Now()
	bytes := p.received()
	p.samples = append(p.samples, speedSample{at: now, bytes: bytes})
	cutoff := now.Add(-time.Second)
	for len(p.samples) > 2 && p.samples[1].at.Before(cutoff) {
		p.samples = p.samples[1:]
	}
	first := p.samples[0]
	p.current.BytesPerSecond = 0
	if seconds := now.Sub(first.at).Seconds(); seconds > 0 && !p.current.Finished {
		p.current.BytesPerSecond = max(0, float64(bytes-first.bytes)/seconds)
	}
	next := p.current
	next.Updates = make([]TaskProgress, 0, len(p.dirty))
	for _, index := range p.dirty {
		next.Updates = append(next.Updates, p.files[index])
	}
	p.notify(next)
	for _, index := range p.dirty {
		p.marked[index] = false
	}
	p.dirty = p.dirty[:0]
}
