package download

import (
	"context"
	"os"
	"path/filepath"

	"github.com/UruhaLushia/tomori/internal/storage"
)

func SpaceRequirements(ctx context.Context, tasks []Task) ([]storage.Requirement, error) {
	var result []storage.Requirement
	directories := make(map[string]int)
	seen := make(map[string]bool)
	for _, t := range tasks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if seen[t.Path] {
			continue
		}
		seen[t.Path] = true
		if info, err := os.Lstat(t.Path); err == nil && info.Mode().IsRegular() && t.Size > 0 && info.Size() == t.Size {
			continue
		}
		dir := filepath.Dir(t.Path)
		index, ok := directories[dir]
		if !ok {
			index = len(result)
			directories[dir] = index
			result = append(result, storage.Requirement{Directory: dir})
		}
		need := &result[index]
		need.Bytes = storage.Add(need.Bytes, uint64(max(0, t.Size)))
		need.Unknown = need.Unknown || t.Size <= 0
	}
	return result, nil
}

func (f *Client) SpaceRequirements(ctx context.Context, tasks []Task) ([]storage.Requirement, error) {
	needs, err := SpaceRequirements(ctx, tasks)
	if err != nil {
		return nil, err
	}
	if !f.Segmented || f.Connections <= 1 {
		return needs, nil
	}
	var segments []Task
	for _, task := range tasks {
		if task.Size < max(1<<20, f.SegmentThreshold) {
			continue
		}
		if info, err := os.Lstat(task.Path); err == nil && info.Mode().IsRegular() && info.Size() == task.Size {
			continue
		}
		task.Path = f.segmentedPath(task)
		segments = append(segments, task)
	}
	partial, err := SpaceRequirements(ctx, segments)
	return append(needs, partial...), err
}

func (f *Client) CheckSpace(ctx context.Context, tasks []Task) (storage.Report, error) {
	needs, err := f.SpaceRequirements(ctx, tasks)
	if err != nil {
		return storage.Report{}, err
	}
	report, err := storage.Assess(ctx, needs)
	if err == nil {
		err = report.Check()
	}
	return report, err
}
