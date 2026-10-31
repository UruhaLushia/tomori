package instancecontent

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

type WorldPacks struct {
	Enabled, Disabled []string
	Version           string
}

func (p WorldPacks) IsEnabled(file string) bool {
	id := "file/" + file
	return slices.Contains(p.Enabled, id) || !slices.Contains(p.Disabled, id)
}

func (n *worldNBT) packList(kind byte, name string) error {
	start := n.offset()
	child, err := n.r.ReadByte()
	if err != nil {
		return err
	}
	size, err := n.number(4)
	if err != nil {
		return err
	}
	if kind != 9 || size < 0 || size > 4096 || size > int64(n.tags) || child != 8 && (size != 0 || child != 0) {
		return errors.New("invalid level.dat datapack list")
	}
	values := make([]string, 0, size)
	for range size {
		n.tags--
		value, err := n.text()
		if err != nil {
			return err
		}
		values = append(values, value)
	}
	if name == "Enabled" {
		if n.enabledStart >= 0 {
			n.duplicate = true
		}
		n.enabledStart, n.enabledEnd, n.packs.Enabled = start, n.offset(), values
	} else {
		if n.disabledStart >= 0 {
			n.duplicate = true
		}
		n.disabledStart, n.disabledEnd, n.packs.Disabled = start, n.offset(), values
	}
	return nil
}

func ReadWorldPacks(ctx context.Context, root *os.Root) (WorldPacks, error) {
	f, err := fsutil.OpenRootRegular(root, "level.dat")
	if err != nil {
		return WorldPacks{}, err
	}
	defer f.Close()
	n, err := parseWorld(ctx, f, true)
	if err == nil && n.duplicate {
		err = errors.New(locales.CouldNotReadWorldDetails())
	}
	n.packs.Version = n.world.Version
	return n.packs, err
}

// Values are 1 to enable, -1 to disable, and 0 to forget a removed file.
func ChangeWorldPacks(ctx context.Context, root *os.Root, changes map[string]int) error {
	return updateWorldPacks(ctx, root, changes, nil)
}

func ReplaceWorldPacks(ctx context.Context, root *os.Root, changes map[string]int, renames map[string]string) error {
	return updateWorldPacks(ctx, root, changes, renames)
}

func updateWorldPacks(ctx context.Context, root *os.Root, changes map[string]int, renames map[string]string) error {
	if len(changes) == 0 && len(renames) == 0 {
		return ctx.Err()
	}
	if len(changes) > 1000 || len(renames) > 1000 {
		return errors.New(locales.InvalidResourceUpdateCountOrLimitExceeded())
	}
	names := make([]string, 0, len(changes))
	for file, value := range changes {
		if err := fsutil.ValidateID(file); err != nil {
			return err
		}
		if strings.ContainsAny(file, "/\\") || value < -1 || value > 1 {
			return errors.New(locales.InvalidResourcePath())
		}
		names = append(names, file)
	}
	slices.Sort(names)
	targets := make(map[string]bool, len(renames))
	for old, next := range renames {
		if err := fsutil.ValidateID(old); err != nil {
			return err
		}
		if err := fsutil.ValidateID(next); err != nil {
			return err
		}
		if targets[next] {
			return errors.New(locales.ResourceUpdateContainsDuplicateFiles())
		}
		targets[next] = true
	}
	f, err := fsutil.OpenRootRegular(root, "level.dat")
	if err != nil {
		return err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return err
	}
	n, err := parseWorld(ctx, f, true)
	if err != nil {
		return err
	}
	if n.duplicate || n.dataEnd < 0 || n.packsSeen && n.packsEnd < 0 {
		return errors.New(locales.CouldNotReadWorldDetails())
	}
	if n.enabledStart < 0 {
		n.packs.Enabled = []string{"vanilla"}
	}
	for _, list := range [][]string{n.packs.Enabled, n.packs.Disabled} {
		for i, id := range list {
			if file, ok := strings.CutPrefix(id, "file/"); ok {
				if next, exists := renames[file]; exists {
					list[i] = "file/" + next
				}
			}
		}
	}
	for _, file := range names {
		id, value := "file/"+file, changes[file]
		if value == 1 && slices.Contains(n.packs.Enabled, id) && !slices.Contains(n.packs.Disabled, id) || value == -1 && slices.Contains(n.packs.Disabled, id) && !slices.Contains(n.packs.Enabled, id) {
			continue
		}
		n.packs.Enabled = slices.DeleteFunc(n.packs.Enabled, func(v string) bool { return v == id })
		n.packs.Disabled = slices.DeleteFunc(n.packs.Disabled, func(v string) bool { return v == id })
		switch value {
		case 1:
			n.packs.Enabled = append(n.packs.Enabled, id)
		case -1:
			n.packs.Disabled = append(n.packs.Disabled, id)
		}
	}
	var edits []worldEdit
	var added []byte
	for _, list := range []struct {
		name       string
		values     []string
		start, end int64
	}{{"Enabled", n.packs.Enabled, n.enabledStart, n.enabledEnd}, {"Disabled", n.packs.Disabled, n.disabledStart, n.disabledEnd}} {
		if len(list.values) > 4096 {
			return errors.New(locales.InvalidResourceUpdateCountOrLimitExceeded())
		}
		raw := make([]byte, 5)
		raw[0] = 8
		binary.BigEndian.PutUint32(raw[1:], uint32(len(list.values)))
		for _, value := range list.values {
			text := encodeWorldText(value)
			if len(text)-2 > 65535 {
				return errors.New(locales.WorldMetadataExceedsTheSizeLimit())
			}
			raw = append(raw, text...)
		}
		if list.start >= 0 {
			edits = append(edits, worldEdit{list.start, list.end, raw})
		} else {
			added = append(added, 9)
			added = append(added, encodeWorldText(list.name)...)
			added = append(added, raw...)
		}
	}
	if len(added) > 0 {
		end := n.packsEnd
		if !n.packsSeen {
			end = n.dataEnd
			added = append(append([]byte{10}, encodeWorldText("DataPacks")...), added...)
			added = append(added, 0)
		}
		edits = append(edits, worldEdit{end, end, added})
	}
	return rewriteWorld(ctx, root, f, before, n, edits)
}
