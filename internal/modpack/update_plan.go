package modpack

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"maps"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

type UpdateKind uint8

const (
	UpdateAdd UpdateKind = iota
	UpdateReplace
	UpdateRemove
)

type UpdateReason uint8

const (
	UpdateUnmodified UpdateReason = iota
	UpdateAlreadyCurrent
	UpdateModified
	UpdateUserAdded
	UpdateUserDeleted
	UpdatePersonal
	UpdateUnsafe
	UpdateDisabledChange
)

type UpdateChoice uint8

const (
	UpdateKeep UpdateChoice = iota
	UpdateUsePack
)

type UpdateEntry struct {
	Key, Path, Target string
	Kind              UpdateKind
	Reason            UpdateReason
	Previous, Next    InstalledFile
	CurrentSHA1       string
	CurrentSize       int64
	Disabled          bool
	Default           UpdateChoice
}

type updateCurrent struct {
	relative string
	hash     string
	info     os.FileInfo
	unsafe   bool
	disabled bool
}

type updateFile struct {
	key, previousKey string
	kind             UpdateKind
	reason           UpdateReason
	current          updateCurrent
}

type UpdatePlan struct {
	previous, next *Origin
	root           os.FileInfo
	files          []updateFile
}

func PlanUpdate(ctx context.Context, root *os.Root, previous, next *Origin, permit func(context.Context) (func(), error), progress func(int, int)) (*UpdatePlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, origin := range []*Origin{previous, next} {
		if origin == nil || origin.Header.validate() != nil || origin.Header.FileCount != len(origin.files) {
			return nil, invalidOrigin()
		}
	}
	previous, next = snapshotOrigin(previous), snapshotOrigin(next)
	stamp, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	p := &UpdatePlan{previous: previous, next: next, root: stamp}
	for key, before := range previous.files {
		after, exists := next.files[key]
		if exists && before.Path == after.Path && before.Size == after.Size && strings.EqualFold(before.SHA1, after.SHA1) {
			continue
		}
		kind := UpdateReplace
		if !exists {
			kind = UpdateRemove
		}
		p.files = append(p.files, updateFile{key: key, previousKey: key, kind: kind})
	}
	for key := range next.files {
		if _, exists := previous.files[key]; !exists {
			p.files = append(p.files, updateFile{key: key, kind: UpdateAdd})
		}
	}
	p.pairRenames()
	slices.SortFunc(p.files, func(a, b updateFile) int { return strings.Compare(a.key, b.key) })
	err = updateWorkers(ctx, len(p.files), permit, progress, func(ctx context.Context, i int) error {
		file := &p.files[i]
		before, after := p.before(*file), next.files[file.key]
		name := before.Path
		if name == "" {
			name = after.Path
		}
		if before.Path != after.Path && strings.EqualFold(before.Path, after.Path) || p.structural(file.key) || file.previousKey != "" && p.structural(file.previousKey) {
			file.reason, file.current.unsafe = UpdateUnsafe, true
			return nil
		}
		current, err := readUpdateCurrent(ctx, root, name)
		if err != nil {
			return err
		}
		file.current = current
		if before.Path != "" && after.Path != "" && before.Path != after.Path {
			target, err := readUpdateCurrent(ctx, root, after.Path)
			if err != nil {
				return err
			}
			if target.unsafe || target.info != nil {
				file.reason = UpdateUnsafe
				if current.info == nil && !target.unsafe && target.hash == after.SHA1 && target.info.Size() == after.Size {
					file.current, file.reason = target, UpdateAlreadyCurrent
				}
				return nil
			}
		}
		file.reason = updateReason(before, after, current)
		return nil
	})
	if err != nil {
		return nil, err
	}
	p.flagDisabledChanges()
	return p, nil
}

func (p *UpdatePlan) before(file updateFile) InstalledFile { return p.previous.files[file.previousKey] }

func (p *UpdatePlan) pairRenames() {
	unique := func(origin *Origin) map[int64]string {
		projects := make(map[int64]string)
		for key, file := range origin.files {
			if file.ProjectID <= 0 {
				continue
			}
			if _, found := projects[file.ProjectID]; found {
				projects[file.ProjectID] = ""
			} else {
				projects[file.ProjectID] = key
			}
		}
		return projects
	}
	before, after := unique(p.previous), unique(p.next)
	removed := make(map[string]int)
	for i, file := range p.files {
		if file.kind == UpdateRemove {
			removed[file.key] = i
		}
	}
	paired := make(map[int]bool)
	for i := range p.files {
		file := &p.files[i]
		next := p.next.files[file.key]
		oldKey := before[next.ProjectID]
		if file.kind != UpdateAdd || oldKey == "" || after[next.ProjectID] != file.key || path.Dir(oldKey) != path.Dir(file.key) {
			continue
		}
		j, found := removed[oldKey]
		if found {
			file.kind, file.previousKey = UpdateReplace, oldKey
			paired[j] = true
		}
	}
	kept := p.files[:0]
	for i, file := range p.files {
		if !paired[i] {
			kept = append(kept, file)
		}
	}
	clear(p.files[len(kept):])
	p.files = kept
}

func (p *UpdatePlan) flagDisabledChanges() {
	type status struct{ disabled, disabledUnknown, added, addedUnknown bool }
	directories := make(map[string]status)
	disabledProjects, addedProjects := make(map[int64]bool), make(map[int64]bool)
	for _, file := range p.files {
		if file.reason != UpdateUnmodified {
			continue
		}
		dir := path.Dir(file.key)
		if !slices.Contains([]string{"mods", "resourcepacks", "shaderpacks"}, dir) {
			continue
		}
		state := directories[dir]
		if file.kind == UpdateRemove && file.current.disabled {
			state.disabled = true
			state.disabledUnknown = state.disabledUnknown || p.before(file).ProjectID == 0
			if project := p.before(file).ProjectID; project > 0 {
				disabledProjects[project] = true
			}
		}
		if file.kind == UpdateAdd {
			state.added = true
			state.addedUnknown = state.addedUnknown || p.next.files[file.key].ProjectID == 0
			if project := p.next.files[file.key].ProjectID; project > 0 {
				addedProjects[project] = true
			}
		}
		directories[dir] = state
	}
	for i := range p.files {
		file := &p.files[i]
		if file.reason != UpdateUnmodified {
			continue
		}
		state := directories[path.Dir(file.key)]
		if file.kind == UpdateAdd && (state.disabledUnknown || state.disabled && p.next.files[file.key].ProjectID == 0 || disabledProjects[p.next.files[file.key].ProjectID]) || file.kind == UpdateRemove && file.current.disabled && (state.addedUnknown || state.added && p.before(*file).ProjectID == 0 || addedProjects[p.before(*file).ProjectID]) {
			file.reason = UpdateDisabledChange
		}
	}
}

func (p *UpdatePlan) Len() int { return len(p.files) }

func (p *UpdatePlan) Previous() OriginHeader { return snapshotHeader(p.previous.Header) }
func (p *UpdatePlan) Next() OriginHeader     { return snapshotHeader(p.next.Header) }

func snapshotHeader(header OriginHeader) OriginHeader {
	header.Pack.Loaders = slices.Clone(header.Pack.Loaders)
	return header
}

func snapshotOrigin(origin *Origin) *Origin {
	copy := *origin
	copy.Header = snapshotHeader(origin.Header)
	copy.files = maps.Clone(origin.files)
	return &copy
}

func (p *UpdatePlan) Entry(i int) UpdateEntry {
	file := p.files[i]
	before, after := p.before(file), p.next.files[file.key]
	name := after.Path
	if name == "" {
		name = before.Path
	}
	target := name
	if file.current.disabled {
		if after.Path == "" {
			target = file.current.relative
		} else if path.Base(path.Dir(file.current.relative)) == ".tomori-disabled" {
			target = path.Dir(after.Path) + "/.tomori-disabled/" + path.Base(after.Path)
		} else if !strings.HasSuffix(target, ".disabled") {
			target += ".disabled"
		}
	}
	entry := UpdateEntry{Key: file.key, Path: name, Target: target, Kind: file.kind, Reason: file.reason, Previous: before, Next: after, CurrentSHA1: file.current.hash, Disabled: file.current.disabled}
	if file.current.info != nil {
		entry.CurrentSize = file.current.info.Size()
	}
	if file.reason == UpdateUnmodified {
		entry.Default = UpdateUsePack
	}
	return entry
}

func (p *UpdatePlan) Validate(ctx context.Context, root *os.Root, choices map[string]UpdateChoice, permit func(context.Context) (func(), error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stamp, err := root.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(stamp, p.root) {
		return updateChanged()
	}
	selected, err := p.selections(choices)
	if err != nil {
		return err
	}
	return updateWorkers(ctx, len(selected), permit, nil, func(ctx context.Context, index int) error {
		i := selected[index]
		entry := p.Entry(i)
		name := entry.Previous.Path
		if name == "" {
			name = entry.Next.Path
		}
		current, err := readUpdateCurrent(ctx, root, name)
		if err != nil {
			return err
		}
		old := p.files[i].current
		if current.unsafe || current.relative != old.relative || current.hash != old.hash || current.disabled != old.disabled || (current.info == nil) != (old.info == nil) {
			return updateChanged()
		}
		if old.info != nil && (!os.SameFile(old.info, current.info) || old.info.Size() != current.info.Size() || !old.info.ModTime().Equal(current.info.ModTime())) {
			return updateChanged()
		}
		if entry.Previous.Path != "" && entry.Next.Path != "" && entry.Previous.Path != entry.Next.Path {
			target, err := readUpdateCurrent(ctx, root, entry.Next.Path)
			if err != nil {
				return err
			}
			if target.unsafe || target.info != nil {
				return updateChanged()
			}
		}
		return nil
	})
}

func (p *UpdatePlan) selections(choices map[string]UpdateChoice) ([]int, error) {
	if len(choices) > p.Len() {
		return nil, updateChanged()
	}
	for key, choice := range choices {
		i, found := slices.BinarySearchFunc(p.files, key, func(file updateFile, key string) int { return strings.Compare(file.key, strings.ToLower(key)) })
		if !found || key != p.files[i].key || choice > UpdateUsePack || choice == UpdateUsePack && p.files[i].reason == UpdateUnsafe {
			return nil, updateChanged()
		}
	}
	var selected []int
	for i := range p.files {
		entry := p.Entry(i)
		choice, chosen := choices[p.files[i].key]
		if !chosen {
			choice = entry.Default
		}
		if choice == UpdateKeep || entry.Reason == UpdateAlreadyCurrent {
			continue
		}
		selected = append(selected, i)
	}
	return selected, nil
}

func (p *UpdatePlan) structural(key string) bool {
	for _, origin := range []*Origin{p.previous, p.next} {
		for parent := path.Dir(key); parent != "."; parent = path.Dir(parent) {
			if _, exists := origin.files[parent]; exists {
				return true
			}
		}
	}
	prefix := key + "/"
	i := sort.Search(len(p.files), func(i int) bool { return p.files[i].key >= prefix })
	return i < len(p.files) && strings.HasPrefix(p.files[i].key, prefix)
}

func updateReason(before, after InstalledFile, current updateCurrent) UpdateReason {
	if current.unsafe {
		return UpdateUnsafe
	}
	if current.info == nil {
		if before.Path != "" {
			if after.Path == "" {
				return UpdateAlreadyCurrent
			}
			return UpdateUserDeleted
		}
		return UpdateUnmodified
	}
	if personalUpdatePath(before.Path) || personalUpdatePath(after.Path) {
		return UpdatePersonal
	}
	if after.Path != "" && current.hash == after.SHA1 && current.info.Size() == after.Size {
		return UpdateAlreadyCurrent
	}
	if before.Path == "" {
		return UpdateUserAdded
	}
	if current.hash != before.SHA1 || current.info.Size() != before.Size {
		return UpdateModified
	}
	return UpdateUnmodified
}

func personalUpdatePath(name string) bool {
	first, _, _ := strings.Cut(strings.ToLower(name), "/")
	return slices.Contains([]string{"saves", "backups", "screenshots", "logs", "crash-reports", "servers.dat", "servers.dat_old", "options.txt", "optionsof.txt", "optionsshaders.txt", "usercache.json", "usernamecache.json", "accounts.json", "launcher_settings.json", "launcher_profiles_microsoft_store.json", "authlib-injector.json", "hmcl.json", "hmcl_profiles.json", "config.db", "config.db-wal", "config.db-shm"}, first)
}

func readUpdateCurrent(ctx context.Context, root *os.Root, name string) (updateCurrent, error) {
	current := updateCurrent{relative: name}
	if len(name) > 4096 || strings.Count(name, "/") >= 128 || validContentPath(name) != nil {
		return current, invalidOrigin()
	}
	names := []string{name}
	first, rest, _ := strings.Cut(name, "/")
	if !strings.Contains(rest, "/") && rest != "" && !strings.HasSuffix(rest, ".disabled") && slices.Contains([]string{"mods", "resourcepacks", "shaderpacks"}, strings.ToLower(first)) {
		names = append(names, name+".disabled", first+"/.tomori-disabled/"+rest)
	}
	for _, relative := range names {
		blocked, missing, err := updateParents(root, relative)
		if err != nil {
			return current, err
		}
		if blocked {
			current.unsafe = true
			return current, nil
		}
		if missing {
			continue
		}
		info, err := root.Lstat(relative)
		if errors.Is(err, os.ErrNotExist) || relative != name && fsutil.IsNameTooLong(err) {
			continue
		}
		if err != nil {
			return current, err
		}
		if current.info != nil || !info.Mode().IsRegular() || info.Size() > 4<<30 {
			current.unsafe = true
			return current, nil
		}
		in, err := fsutil.OpenRootRegular(root, relative)
		if err != nil {
			return current, err
		}
		hash := sha1.New()
		buffer := archiveBuffers.Get().(*[]byte)
		n, copyErr := io.CopyBuffer(hash, io.LimitReader(fsutil.ContextReader(ctx, in), info.Size()+1), *buffer)
		archiveBuffers.Put(buffer)
		after, statErr := in.Stat()
		_ = in.Close()
		if copyErr != nil || statErr != nil {
			return current, errors.Join(copyErr, statErr)
		}
		latest, err := root.Lstat(relative)
		if err != nil {
			return current, err
		}
		if n != info.Size() || !sameUpdateFile(info, after) || !sameUpdateFile(info, latest) {
			return current, updateChanged()
		}
		current.relative, current.info, current.hash = relative, after, hex.EncodeToString(hash.Sum(nil))
		current.disabled = relative != name || strings.HasSuffix(name, ".disabled")
	}
	return current, nil
}

func updateParents(root *os.Root, name string) (blocked, missing bool, err error) {
	for i, ch := range name {
		if ch != '/' {
			continue
		}
		info, err := root.Lstat(name[:i])
		if errors.Is(err, os.ErrNotExist) {
			return false, true, nil
		}
		if err != nil {
			return false, false, err
		}
		if !info.IsDir() {
			return true, false, nil
		}
	}
	return false, false, nil
}

func sameUpdateFile(a, b os.FileInfo) bool {
	return a != nil && b != nil && b.Mode().IsRegular() && a.Mode() == b.Mode() && os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

func updateChanged() error {
	return errors.New(locales.InstanceFilesChangedCheckTheModpackUpdateAgain())
}

func updateWorkers(ctx context.Context, count int, permit func(context.Context) (func(), error), progress func(int, int), work func(context.Context, int) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	var workers sync.WaitGroup
	var mu sync.Mutex
	var problem error
	done := 0
	var last time.Time
	for range min(4, count) {
		workers.Go(func() {
			for i := range jobs {
				var err error
				var release func()
				if permit != nil {
					release, err = permit(ctx)
				}
				if err == nil {
					err = work(ctx, i)
				}
				if release != nil {
					release()
				}
				mu.Lock()
				if err != nil {
					if problem == nil {
						problem = err
						cancel()
					}
				} else {
					done++
					if progress != nil && (done == count || time.Since(last) >= 100*time.Millisecond) {
						last = time.Now()
						progress(done, count)
					}
				}
				mu.Unlock()
			}
		})
	}
send:
	for i := range count {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break send
		}
	}
	close(jobs)
	workers.Wait()
	if problem != nil {
		return problem
	}
	return ctx.Err()
}
