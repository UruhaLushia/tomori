package fsutil

import (
	"context"
	"io"
	"io/fs"
)

func ReadExact(ctx context.Context, in io.Reader, size int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if size < 0 || int64(int(size)) != size {
		return nil, fs.ErrInvalid
	}
	r := &contextReader{ctx, in}
	var data []byte
	if size <= 8<<20 {
		data = make([]byte, int(size))
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}
	} else {
		var err error
		data, err = io.ReadAll(io.LimitReader(r, size))
		if err != nil {
			return nil, err
		}
		if int64(len(data)) != size {
			return nil, io.ErrUnexpectedEOF
		}
	}
	var extra [1]byte
	_, err := io.ReadFull(r, extra[:])
	if err == io.EOF {
		return data, nil
	}
	if err == nil {
		err = io.ErrUnexpectedEOF
	}
	return nil, err
}
