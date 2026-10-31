package fsutil

import (
	"context"
	"errors"
	"os"

	"github.com/UruhaLushia/tomori/locales"
)

func ReadRootFile(root *os.Root, name string, limit int64) ([]byte, error) {
	f, err := OpenRootRegular(root, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || info.Size() > limit {
		return nil, errors.New(locales.InvalidInstanceFileTypeOrSize())
	}
	return ReadExact(context.Background(), f, info.Size())
}
