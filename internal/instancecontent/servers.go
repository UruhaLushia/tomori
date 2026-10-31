package instancecontent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/serverstatus"
	"github.com/UruhaLushia/tomori/locales"
)

const serverListLimit = 4 << 20

type Server struct {
	Name, Address string
	Textures      int
}

type ServerList struct {
	Entries  []Server
	Revision [32]byte
}

type serverRecord struct {
	start, end int64
	fields     []worldEdit
}

type serverDocument struct {
	ServerList
	raw        []byte
	info       os.FileInfo
	start, end int64
	records    []serverRecord
}

func ReadServers(ctx context.Context, directory string) (ServerList, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return ServerList{}, err
	}
	defer root.Close()
	doc, err := readServerDocument(ctx, root)
	return doc.ServerList, err
}

func readServerDocument(ctx context.Context, root *os.Root) (serverDocument, error) {
	d := serverDocument{start: -1}
	f, err := fsutil.OpenRootRegular(root, "servers.dat")
	if errors.Is(err, os.ErrNotExist) {
		d.Revision = sha256.Sum256(nil)
		d.raw, d.end = []byte{10, 0, 0, 0}, 3
		return d, ctx.Err()
	}
	if err != nil {
		return d, err
	}
	defer f.Close()
	d.info, err = f.Stat()
	if err != nil {
		return d, err
	}
	if d.info.Size() > serverListLimit {
		return d, errors.New(locales.ServerListExceedsTheSizeLimit())
	}
	d.raw, err = io.ReadAll(io.LimitReader(&worldReader{ctx: ctx, Reader: f}, serverListLimit+1))
	if err != nil {
		return d, err
	}
	if len(d.raw) > serverListLimit {
		return d, errors.New(locales.ServerListExceedsTheSizeLimit())
	}
	d.Revision = sha256.Sum256(d.raw)
	n := worldNBT{ctx: ctx, tags: 100000, source: &worldReader{ctx: ctx, Reader: bytes.NewReader(d.raw)}}
	n.r = bufio.NewReaderSize(n.source, 4096)
	kind, err := n.r.ReadByte()
	if err != nil || kind != 10 {
		return d, errors.New(locales.InvalidServerList())
	}
	if _, err := n.text(); err != nil {
		return d, err
	}
	for {
		end := n.offset()
		kind, err := n.r.ReadByte()
		if err != nil {
			return d, err
		}
		if kind == 0 {
			if d.start < 0 {
				d.end = end
			}
			break
		}
		name, err := n.text()
		if err != nil {
			return d, err
		}
		if name != "servers" {
			if err := n.payload(kind, worldOther, 0); err != nil {
				return d, err
			}
			continue
		}
		if kind != 9 || d.start >= 0 {
			return d, errors.New(locales.InvalidServerList())
		}
		d.start = n.offset()
		child, err := n.r.ReadByte()
		if err != nil {
			return d, err
		}
		count, err := n.number(4)
		if err != nil || count < 0 || count > 512 || child != 10 && (child != 0 || count != 0) {
			return d, errors.New(locales.InvalidServerList())
		}
		for range count {
			entry, record, err := readServerRecord(&n)
			if err != nil {
				return d, err
			}
			d.Entries, d.records = append(d.Entries, entry), append(d.records, record)
		}
		d.end = n.offset()
	}
	if n.offset() != int64(len(d.raw)) {
		return d, errors.New(locales.InvalidServerList())
	}
	return d, ctx.Err()
}

func readServerRecord(n *worldNBT) (Server, serverRecord, error) {
	s, record := Server{}, serverRecord{start: n.offset()}
	seen := make(map[string]bool, 3)
	for {
		start := n.offset()
		kind, err := n.r.ReadByte()
		if err != nil {
			return s, record, err
		}
		if kind == 0 {
			record.end = n.offset()
			return s, record, nil
		}
		name, err := n.text()
		if err != nil {
			return s, record, err
		}
		switch name {
		case "name", "ip", "acceptTextures":
			if seen[name] || name == "acceptTextures" && kind != 1 || name != "acceptTextures" && kind != 8 {
				return s, record, errors.New(locales.InvalidServerList())
			}
			seen[name] = true
			if name == "acceptTextures" {
				value, err := n.number(1)
				if err != nil {
					return s, record, err
				}
				s.Textures = 1
				if value == 0 {
					s.Textures = 2
				}
			} else {
				value, err := n.text()
				if err != nil {
					return s, record, err
				}
				if len(value) > 4096 {
					return s, record, errors.New(locales.ServerListExceedsTheSizeLimit())
				}
				if name == "name" {
					s.Name = value
				} else {
					s.Address = value
				}
			}
			record.fields = append(record.fields, worldEdit{start: start, end: n.offset()})
		default:
			if err := n.payload(kind, worldOther, 0); err != nil {
				return s, record, err
			}
		}
	}
}

func ChangeServer(ctx context.Context, directory string, revision [32]byte, index int, next Server, remove bool) error {
	if !remove {
		next.Name, next.Address = strings.TrimSpace(next.Name), strings.TrimSpace(next.Address)
		if next.Name == "" || !utf8.ValidString(next.Name) || utf8.RuneCountInString(next.Name) > 128 || strings.ContainsAny(next.Name, "\x00\r\n") || next.Textures < 0 || next.Textures > 2 {
			return errors.New(locales.EnterAValidServerName())
		}
		if _, err := serverstatus.ParseAddress(next.Address); err != nil {
			return err
		}
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	d, err := readServerDocument(ctx, root)
	if err != nil {
		return err
	}
	if d.Revision != revision || index < -1 || index >= len(d.Entries) || index == -1 && remove {
		return errors.New(locales.ServerListChangedRefreshAndTryAgain())
	}
	count := len(d.Entries)
	if index == -1 {
		count++
	} else if remove {
		count--
	}
	if count > 512 {
		return errors.New(locales.UpTo512ServersCanBeSaved())
	}
	var replacement []byte
	if !remove {
		if index >= 0 {
			record := d.records[index]
			cursor := record.start
			for _, field := range record.fields {
				replacement = append(replacement, d.raw[cursor:field.start]...)
				cursor = field.end
			}
			replacement = append(replacement, d.raw[cursor:record.end-1]...)
		}
		for _, field := range []struct{ name, value string }{{"name", next.Name}, {"ip", next.Address}} {
			replacement = append(replacement, 8)
			replacement = append(replacement, encodeWorldText(field.name)...)
			replacement = append(replacement, encodeWorldText(field.value)...)
		}
		if next.Textures != 0 {
			replacement = append(replacement, 1)
			replacement = append(replacement, encodeWorldText("acceptTextures")...)
			replacement = append(replacement, byte(2-next.Textures))
		}
		replacement = append(replacement, 0)
	}
	start := d.start
	if start < 0 {
		start = d.end
	}
	size := int64(len(d.raw)) - (d.end - start) + 5 + int64(len(replacement))
	if d.start < 0 {
		size += int64(1 + len(encodeWorldText("servers")))
	}
	for i, record := range d.records {
		if i != index {
			size += record.end - record.start
		}
	}
	if size > serverListLimit {
		return errors.New(locales.ServerListExceedsTheSizeLimit())
	}
	return fsutil.WriteRootAtomicStream(ctx, root, "servers.dat", func(w io.Writer) error {
		out := w
		if _, err := out.Write(d.raw[:start]); err != nil {
			return err
		}
		if d.start < 0 {
			if _, err := out.Write(append([]byte{9}, encodeWorldText("servers")...)); err != nil {
				return err
			}
		}
		header := [5]byte{10}
		binary.BigEndian.PutUint32(header[1:], uint32(count))
		if _, err := out.Write(header[:]); err != nil {
			return err
		}
		for i, record := range d.records {
			raw := d.raw[record.start:record.end]
			if i == index {
				raw = replacement
			}
			if _, err := out.Write(raw); err != nil {
				return err
			}
		}
		if index == -1 {
			if _, err := out.Write(replacement); err != nil {
				return err
			}
		}
		if _, err := out.Write(d.raw[d.end:]); err != nil {
			return err
		}
		original, err := root.Stat(".")
		current, currentErr := os.Stat(directory)
		if err != nil || currentErr != nil || !os.SameFile(original, current) {
			return errors.New(locales.ServerListChangedRefreshAndTryAgain())
		}
		current, err = root.Lstat("servers.dat")
		if d.info == nil && !errors.Is(err, os.ErrNotExist) || d.info != nil && (err != nil || !os.SameFile(d.info, current) || current.Size() != d.info.Size() || !current.ModTime().Equal(d.info.ModTime())) {
			return errors.New(locales.ServerListChangedRefreshAndTryAgain())
		}
		return ctx.Err()
	})
}
