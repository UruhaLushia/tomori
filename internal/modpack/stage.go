package modpack

import (
	"context"
	"os"
	"path/filepath"
	"sync"
)

func StageFiles(ctx context.Context, destination *os.Root, files []File, sources []string, permit func(context.Context) (func(), error), record func(InstalledFile) error) error {
	if len(files) != len(sources) || len(files) > 200000 {
		return invalidOrigin()
	}
	var mu sync.Mutex
	return updateWorkers(ctx, len(files), permit, nil, func(ctx context.Context, index int) error {
		file := files[index]
		if err := updateContentPath(file.Path); err != nil {
			return err
		}
		source, err := os.OpenRoot(filepath.Dir(sources[index]))
		if err != nil {
			return err
		}
		defer source.Close()
		if _, err := copyUpdateFile(ctx, source, filepath.Base(sources[index]), destination, file.Path, file.SHA1, file.Size, nil); err != nil {
			return err
		}
		if record == nil {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		return record(InstalledFile{Path: file.Path, SHA1: file.SHA1, Size: file.Size, ProjectID: file.ProjectID, FileID: file.FileID, Optional: file.Optional})
	})
}
