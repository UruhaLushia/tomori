package modpack

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

type MetadataFile struct {
	Path, ExpectedSHA1    string
	Remove, CheckExpected bool
}

type MetadataPlan struct {
	root, source os.FileInfo
	files        []updateOperation
	sources      []os.FileInfo
}

func invalidMetadata() error { return errors.New(locales.InvalidOrOversizedInstanceConfiguration()) }

func metadataPath(name string) error {
	if name == "" || len(name) > 4096 || strings.Contains(name, "\\") || strings.Count(name, "/") >= 128 {
		return invalidMetadata()
	}
	for part := range strings.SplitSeq(name, "/") {
		if fsutil.ValidateID(part) != nil || part == "." || part == ".." || strings.HasPrefix(strings.ToLower(part), ".tomori-pack-update-") || strings.HasPrefix(strings.ToLower(part), ".tomori-pack-config-") || strings.HasPrefix(strings.ToLower(part), ".tomori-write-") {
			return invalidMetadata()
		}
	}
	if !strings.HasSuffix(strings.ToLower(name), ".json") && name != "instance.cfg" {
		return invalidMetadata()
	}
	return nil
}

func metadataInfo(root *os.Root, name string, required bool) (os.FileInfo, error) {
	blocked, missing, err := updateParents(root, name)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, updateChanged()
	}
	if missing && !required {
		return nil, nil
	}
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) && !required {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 128<<20 {
		return nil, invalidMetadata()
	}
	return info, nil
}

func PlanMetadata(ctx context.Context, root, candidate *os.Root, files []MetadataFile, permit func(context.Context) (func(), error)) (*MetadataPlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if root == nil || len(files) == 0 || len(files) > 64 {
		return nil, invalidMetadata()
	}
	files = slices.Clone(files)
	slices.SortFunc(files, func(a, b MetadataFile) int { return strings.Compare(strings.ToLower(a.Path), strings.ToLower(b.Path)) })
	plan := &MetadataPlan{files: make([]updateOperation, len(files)), sources: make([]os.FileInfo, len(files))}
	var err error
	plan.root, err = root.Stat(".")
	if err != nil {
		return nil, err
	}
	if candidate != nil {
		plan.source, err = candidate.Stat(".")
		if err != nil {
			return nil, err
		}
		if err := updateRootCurrent(candidate, plan.source); err != nil {
			return nil, err
		}
	}
	if err := updateRootCurrent(root, plan.root); err != nil {
		return nil, err
	}
	var total int64
	for i, input := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := metadataPath(input.Path); err != nil {
			return nil, err
		}
		if !input.CheckExpected && input.ExpectedSHA1 != "" || input.ExpectedSHA1 != "" && !validHash(input.ExpectedSHA1, 20) {
			return nil, invalidMetadata()
		}
		if i > 0 && strings.EqualFold(input.Path, files[i-1].Path) {
			return nil, invalidMetadata()
		}
		file := &plan.files[i]
		file.target = input.Path
		file.previous.info, err = metadataInfo(root, input.Path, false)
		if err != nil {
			return nil, err
		}
		if file.previous.info != nil {
			file.old = input.Path
			total += file.previous.info.Size()
		}
		if !input.Remove {
			if candidate == nil {
				return nil, invalidMetadata()
			}
			plan.sources[i], err = metadataInfo(candidate, input.Path, true)
			if err != nil {
				return nil, err
			}
			file.next.Path = input.Path
			total += plan.sources[i].Size()
		}
		if total > 512<<20 {
			return nil, invalidMetadata()
		}
	}
	err = updateWorkers(ctx, len(plan.files), permit, nil, func(ctx context.Context, i int) error {
		file := &plan.files[i]
		if file.previous.info != nil {
			var err error
			file.previous, err = updateHash(ctx, root, file.target, file.previous.info)
			if err != nil {
				return err
			}
		}
		if input := files[i]; input.CheckExpected && !strings.EqualFold(input.ExpectedSHA1, file.previous.hash) {
			return updateChanged()
		}
		if file.next.Path != "" {
			current, err := updateHash(ctx, candidate, file.next.Path, plan.sources[i])
			if err != nil {
				return err
			}
			file.next.SHA1, file.next.Size = current.hash, current.size
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := updateRootCurrent(root, plan.root); err != nil {
		return nil, err
	}
	if candidate != nil {
		if err := updateRootCurrent(candidate, plan.source); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

func (p *MetadataPlan) Validate(ctx context.Context, root *os.Root, permit func(context.Context) (func(), error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p == nil || root == nil {
		return invalidMetadata()
	}
	if err := updateRootCurrent(root, p.root); err != nil {
		return err
	}
	return updateWorkers(ctx, len(p.files), permit, nil, func(ctx context.Context, i int) error {
		file := p.files[i]
		info, err := metadataInfo(root, file.target, false)
		if err != nil {
			return err
		}
		if file.previous.info == nil {
			if info != nil {
				return updateChanged()
			}
			return nil
		}
		if !sameUpdateFile(file.previous.info, info) {
			return updateChanged()
		}
		current, err := updateHash(ctx, root, file.old, file.previous.info)
		if err != nil {
			return err
		}
		if current.hash != file.previous.hash {
			return updateChanged()
		}
		return nil
	})
}

func (t *UpdateTransaction) ApplyMetadata(ctx context.Context, plan *MetadataPlan, root, candidate *os.Root) (err error) {
	if t.finished || t.updateSet == nil || t.root == nil || t.control != nil {
		return errors.New(locales.ModpackUpdateTransactionHasEnded())
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, t.Rollback())
		}
	}()
	if err := plan.Validate(ctx, root, t.permit); err != nil {
		return err
	}
	space, err := plan.assessSpace(ctx, root)
	if err != nil {
		return err
	}
	if err := space.Check(); err != nil {
		return err
	}
	if plan.source != nil {
		if candidate == nil {
			return invalidMetadata()
		}
		if err := updateRootCurrent(candidate, plan.source); err != nil {
			return err
		}
	}
	t.control, err = newUpdateSet(root, plan.root, ".tomori-pack-config-", t.permit)
	if err != nil {
		return err
	}
	control := t.control
	control.files = slices.Clone(plan.files)
	if err := control.makeStage(); err != nil {
		return err
	}
	reference := struct {
		Schema int    `json:"schema"`
		Root   string `json:"root"`
		Stage  string `json:"stage"`
	}{1, root.Name(), control.name}
	raw, err := json.Marshal(reference)
	if err != nil {
		return err
	}
	var staged []int
	for i, file := range control.files {
		if file.next.Path != "" {
			staged = append(staged, i)
		}
	}
	err = updateWorkers(ctx, len(staged), t.permit, nil, func(ctx context.Context, index int) error {
		i := staged[index]
		file := &control.files[i]
		name := "next/" + strconv.Itoa(i)
		var err error
		file.staged, err = copyUpdateFile(ctx, candidate, file.next.Path, control.stage, name, file.next.SHA1, file.next.Size, plan.sources[i])
		if err != nil {
			return err
		}
		if err := control.stage.Chmod(name, 0o600); err != nil {
			return err
		}
		file.staged.info, err = control.stage.Lstat(name)
		return err
	})
	if err != nil {
		return err
	}
	if err := control.step(ctx, func() error { return control.writeMetadataManifest(ctx, t.directory) }); err != nil {
		return err
	}
	if err := t.step(ctx, func() error { return fsutil.WriteRootAtomic(ctx, t.stage, "controls.json", raw) }); err != nil {
		return err
	}
	if err := plan.Validate(ctx, root, t.permit); err != nil {
		return err
	}
	for i := range control.files {
		if err := control.step(ctx, func() error { return control.applyFile(ctx, i) }); err != nil {
			return err
		}
	}
	return t.Validate(ctx)
}

func (t *updateSet) writeMetadataManifest(ctx context.Context, parent string) error {
	files := make([]struct {
		Path       string        `json:"path"`
		Old        bool          `json:"old"`
		BeforeSHA1 string        `json:"beforeSHA1,omitempty"`
		BeforeSize int64         `json:"beforeSize,omitempty"`
		BeforeMode uint32        `json:"beforeMode,omitempty"`
		AfterMode  *uint32       `json:"afterMode,omitempty"`
		Next       InstalledFile `json:"next"`
	}, len(t.files))
	for i, file := range t.files {
		files[i].Path, files[i].Old, files[i].BeforeSHA1, files[i].BeforeSize, files[i].Next = file.target, file.old != "", file.previous.hash, file.previous.size, file.next
		if file.previous.info != nil {
			files[i].BeforeMode = uint32(file.previous.info.Mode().Perm())
		}
		if file.staged.info != nil {
			mode := uint32(file.staged.info.Mode().Perm())
			files[i].AfterMode = &mode
		}
	}
	raw, err := json.Marshal(struct {
		Schema    int       `json:"schema"`
		CreatedAt time.Time `json:"createdAt"`
		Parent    string    `json:"parent"`
		Files     any       `json:"files"`
	}{1, time.Now().UTC(), parent, files})
	if err != nil {
		return err
	}
	return fsutil.WriteRootAtomic(ctx, t.stage, "manifest.json", raw)
}
