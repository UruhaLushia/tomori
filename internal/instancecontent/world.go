package instancecontent

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/internal/fsutil"
)

type World struct {
	Valid         bool
	Name, Version string
	Seed          int64
	HasSeed       bool
	Mode          int
	Hardcore      bool
	LastPlayed    time.Time
}

func ReadWorld(ctx context.Context, path string) (World, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return World{}, err
	}
	defer root.Close()
	f, err := fsutil.OpenRootRegular(root, "level.dat")
	if err != nil {
		return World{}, err
	}
	defer f.Close()
	n, err := parseWorld(ctx, f, false)
	return n.world, err
}

func parseWorld(ctx context.Context, f *os.File, packs bool) (worldNBT, error) {
	info, err := f.Stat()
	if err != nil {
		return worldNBT{}, err
	}
	return parseWorldReader(ctx, f, info.Size(), packs)
}

func ReadWorldData(ctx context.Context, raw []byte) (World, error) {
	n, err := parseWorldReader(ctx, bytes.NewReader(raw), int64(len(raw)), false)
	return n.world, err
}

func parseWorldReader(ctx context.Context, in io.Reader, size int64, packs bool) (worldNBT, error) {
	n := worldNBT{ctx: ctx, tags: 100000, nameStart: -1, dataEnd: -1, packsEnd: -1, enabledStart: -1, disabledStart: -1, readPacks: packs}
	if err := ctx.Err(); err != nil {
		return n, err
	}
	if size > 8<<20 {
		return n, errors.New("level.dat size limit")
	}
	z, err := gzip.NewReader(in)
	if err != nil {
		return n, err
	}
	defer z.Close()
	n.source = &worldReader{ctx: ctx, Reader: io.LimitReader(z, (8<<20)+1)}
	n.r = bufio.NewReaderSize(n.source, 4096)
	kind, err := n.r.ReadByte()
	if err != nil {
		return n, err
	}
	if kind != 10 {
		return n, errors.New("invalid level.dat root")
	}
	if _, err = n.text(); err == nil {
		err = n.payload(kind, worldRoot, 0)
	}
	if err == nil {
		_, err = n.r.ReadByte()
		switch err {
		case io.EOF:
			err = nil
		case nil:
			err = errors.New("trailing level.dat data")
		}
	}
	if err == nil && !n.world.Valid {
		err = errors.New("missing level.dat data")
	}
	if err == nil && n.offset() > 8<<20 {
		err = errors.New("level.dat size limit")
	}
	return n, err
}

const (
	worldOther = iota
	worldRoot
	worldData
	worldVersion
	worldGeneration
	worldPacks
)

type worldNBT struct {
	ctx                         context.Context
	r                           *bufio.Reader
	world                       World
	tags                        int
	buf                         [8]byte
	source                      *worldReader
	nameStart, nameEnd, dataEnd int64
	duplicate                   bool
	packs                       WorldPacks
	packsEnd                    int64
	enabledStart, enabledEnd    int64
	disabledStart, disabledEnd  int64
	packsSeen                   bool
	readPacks                   bool
}

type worldReader struct {
	io.Reader
	ctx  context.Context
	read int64
}

func (r *worldReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.Reader.Read(p)
	r.read += int64(n)
	return n, err
}

func (n *worldNBT) offset() int64 { return n.source.read - int64(n.r.Buffered()) }

func (n *worldNBT) number(size int) (int64, error) {
	_, err := io.ReadFull(n.r, n.buf[:size])
	switch size {
	case 1:
		return int64(int8(n.buf[0])), err
	case 4:
		return int64(int32(binary.BigEndian.Uint32(n.buf[:4]))), err
	default:
		return int64(binary.BigEndian.Uint64(n.buf[:])), err
	}
}

func (n *worldNBT) text() (string, error) {
	if _, err := io.ReadFull(n.r, n.buf[:2]); err != nil {
		return "", err
	}
	size := int(binary.BigEndian.Uint16(n.buf[:2]))
	data := make([]byte, size)
	if _, err := io.ReadFull(n.r, data); err != nil {
		return "", err
	}
	if utf8.Valid(data) {
		return string(data), nil
	}
	units := make([]uint16, 0, len(data))
	for len(data) > 0 {
		switch {
		case data[0] < 0x80:
			units, data = append(units, uint16(data[0])), data[1:]
		case len(data) >= 2 && data[0]&0xe0 == 0xc0 && data[1]&0xc0 == 0x80:
			units, data = append(units, uint16(data[0]&0x1f)<<6|uint16(data[1]&0x3f)), data[2:]
		case len(data) >= 3 && data[0]&0xf0 == 0xe0 && data[1]&0xc0 == 0x80 && data[2]&0xc0 == 0x80:
			units, data = append(units, uint16(data[0]&0xf)<<12|uint16(data[1]&0x3f)<<6|uint16(data[2]&0x3f)), data[3:]
		default:
			return "", errors.New("invalid level.dat string")
		}
	}
	return string(utf16.Decode(units)), nil
}

func (n *worldNBT) skip(size int64) error {
	if size < 0 || size > 8<<20 {
		return errors.New("invalid level.dat length")
	}
	_, err := io.CopyN(io.Discard, n.r, size)
	return err
}

func (n *worldNBT) payload(kind byte, scope, depth int) error {
	n.tags--
	if depth > 64 || n.tags < 0 {
		return errors.New("level.dat complexity limit")
	}
	if err := n.ctx.Err(); err != nil {
		return err
	}
	switch kind {
	case 1, 2, 3, 4, 5, 6:
		return n.skip([7]int64{0, 1, 2, 4, 8, 4, 8}[kind])
	case 7, 11, 12:
		size, err := n.number(4)
		if err != nil {
			return err
		}
		width := int64(1)
		switch kind {
		case 11:
			width = 4
		case 12:
			width = 8
		}
		return n.skip(size * width)
	case 8:
		if _, err := io.ReadFull(n.r, n.buf[:2]); err != nil {
			return err
		}
		return n.skip(int64(binary.BigEndian.Uint16(n.buf[:2])))
	case 9:
		child, err := n.r.ReadByte()
		if err != nil {
			return err
		}
		size, err := n.number(4)
		if err != nil {
			return err
		}
		if size < 0 || size > int64(n.tags) || child > 12 || child == 0 && size != 0 {
			return errors.New("invalid level.dat list")
		}
		for range size {
			if err := n.payload(child, worldOther, depth+1); err != nil {
				return err
			}
		}
		return nil
	case 10:
		for {
			start := n.offset()
			child, err := n.r.ReadByte()
			if err != nil || child == 0 {
				if err == nil && scope == worldData {
					n.dataEnd = start
				}
				if err == nil && scope == worldPacks {
					n.packsEnd = start
				}
				return err
			}
			name, err := n.text()
			if err != nil {
				return err
			}
			if err = n.field(child, scope, name, depth); err != nil {
				return err
			}
		}
	}
	return errors.New("invalid level.dat tag")
}

func (n *worldNBT) field(kind byte, scope int, name string, depth int) error {
	n.tags--
	if n.tags < 0 {
		return errors.New("level.dat complexity limit")
	}
	if scope == worldPacks && (name == "Enabled" || name == "Disabled") {
		return n.packList(kind, name)
	}
	if n.readPacks && scope == worldData && name == "DataPacks" && kind != 10 {
		return errors.New("invalid level.dat datapacks")
	}
	if kind == 8 && (scope == worldData && name == "LevelName" || scope == worldVersion && name == "Name") {
		start := n.offset()
		value, err := n.text()
		if len(value) > 1024 {
			end := 1024
			for !utf8.RuneStart(value[end]) {
				end--
			}
			value = value[:end]
		}
		if scope == worldData {
			if n.nameStart >= 0 {
				n.duplicate = true
			}
			n.nameStart, n.nameEnd = start, n.offset()
			n.world.Name = value
		} else {
			n.world.Version = value
		}
		return err
	}
	if kind == 4 && (scope == worldGeneration && name == "seed" || scope == worldData && (name == "RandomSeed" || name == "LastPlayed")) {
		value, err := n.number(8)
		if name == "LastPlayed" {
			if value > 0 {
				n.world.LastPlayed = time.UnixMilli(value)
			}
		} else if scope == worldGeneration || !n.world.HasSeed {
			n.world.Seed, n.world.HasSeed = value, true
		}
		return err
	}
	if scope == worldData && kind == 3 && name == "GameType" {
		value, err := n.number(4)
		n.world.Mode = int(value)
		return err
	}
	if scope == worldData && kind == 1 && name == "hardcore" {
		value, err := n.number(1)
		n.world.Hardcore = value != 0
		return err
	}
	next := worldOther
	if kind == 10 {
		switch {
		case scope == worldRoot && name == "Data":
			if n.world.Valid {
				n.duplicate = true
			}
			next = worldData
			n.world.Valid = true
		case scope == worldData && name == "Version":
			next = worldVersion
		case scope == worldData && name == "WorldGenSettings":
			next = worldGeneration
		case n.readPacks && scope == worldData && name == "DataPacks":
			if n.packsSeen {
				n.duplicate = true
			}
			n.packsSeen, next = true, worldPacks
		}
	}
	return n.payload(kind, next, depth+1)
}
