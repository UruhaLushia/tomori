package download

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/UruhaLushia/tomori/locales"
)

func (f *Client) FetchAll(ctx context.Context, tasks []Task, workers int, progress func(Progress)) error {
	return f.FetchAllWithReuse(ctx, tasks, workers, progress, nil)
}

func (f *Client) FetchAllWithReuse(ctx context.Context, tasks []Task, workers int, progress func(Progress), sources LocalSources) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	pending := make([]Task, 0, len(tasks))
	seen := make(map[string]int, len(tasks))
	for _, t := range tasks {
		if err := ctx.Err(); err != nil {
			return err
		}
		if index, ok := seen[t.Path]; ok {
			previous := pending[index]
			if previous.URL != t.URL || previous.SHA1 != t.SHA1 || previous.SHA512 != t.SHA512 || previous.Size != t.Size || previous.MaxSize != t.MaxSize || !slices.Equal(previous.Mirrors, t.Mirrors) {
				return fmt.Errorf(locales.DownloadTargetConflict(), t.Path)
			}
			continue
		}
		seen[t.Path] = len(pending)
		pending = append(pending, t)
	}
	// Start long transfers early while other workers drain small files.
	if _, err := f.CheckSpace(ctx, pending); err != nil {
		return err
	}
	slices.SortStableFunc(pending, func(a, b Task) int { return cmp.Compare(b.Size, a.Size) })
	report := newProgress(pending, progress, f.received.Load)
	if len(pending) == 0 {
		report.finish(nil)
		return nil
	}
	stopReporting := report.start()
	if workers <= 0 {
		workers = f.Workers
	}
	workers = min(len(pending), max(1, min(workers, 32)))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	queue := make(chan int)
	var wg sync.WaitGroup
	var fail sync.Once
	var firstErr error
	for range workers {
		wg.Go(func() {
			for index := range queue {
				if ctx.Err() != nil {
					return
				}
				task := pending[index]
				report.update(index, 0, TaskChecking, "")
				var notify func(int64)
				if report != nil {
					notify = func(n int64) { report.update(index, n, TaskDownloading, "") }
				}
				err := f.fetchFrom(ctx, task, notify, sources, false)
				if err != nil {
					state := TaskFailed
					if ctx.Err() != nil {
						state = TaskCanceled
					}
					report.update(index, 0, state, err.Error())
					fail.Do(func() { firstErr = err; cancel() })
					return
				}
				report.update(index, task.Size, TaskCompleted, "")
			}
		})
	}
	for index := range pending {
		select {
		case queue <- index:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(queue)
	wg.Wait()
	stopReporting()
	if firstErr == nil {
		firstErr = ctx.Err()
	}
	report.finish(firstErr)
	return firstErr
}
