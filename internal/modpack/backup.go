package modpack

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

type BackupInfo struct {
	Name, From, To, Issue string
	CreatedAt             time.Time
	Files                 int
	Interrupted           bool
}

type backupControlRecord struct {
	Path       string        `json:"path"`
	Old        bool          `json:"old"`
	BeforeSHA1 string        `json:"beforeSHA1,omitempty"`
	BeforeSize int64         `json:"beforeSize,omitempty"`
	BeforeMode uint32        `json:"beforeMode,omitempty"`
	AfterMode  *uint32       `json:"afterMode,omitempty"`
	Next       InstalledFile `json:"next"`
}

type backupControlFiles []backupControlRecord

func (files *backupControlFiles) UnmarshalJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if token, err := d.Token(); err != nil || token != json.Delim('[') {
		return invalidBackup()
	}
	var result backupControlFiles
	for d.More() {
		if len(result) >= 64 {
			return invalidBackup()
		}
		var record backupControlRecord
		if err := d.Decode(&record); err != nil {
			return invalidBackup()
		}
		result = append(result, record)
	}
	if token, err := d.Token(); err != nil || token != json.Delim(']') {
		return invalidBackup()
	}
	if _, err := d.Token(); err != io.EOF {
		return invalidBackup()
	}
	*files = result
	return nil
}

func invalidBackup() error { return errors.New(locales.BackupIsInvalidOrIncomplete()) }

func backupName(name, prefix string) bool {
	return strings.HasPrefix(name, prefix) && len(name) > len(prefix) && fsutil.ValidateID(name) == nil
}

func openBackupRoot(root *os.Root, name string) (*os.Root, os.FileInfo, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, invalidBackup()
	}
	held, err := root.OpenRoot(name)
	if err != nil {
		return nil, nil, err
	}
	stamp, err := held.Stat(".")
	if err != nil || !os.SameFile(info, stamp) {
		return nil, nil, errors.Join(err, held.Close(), invalidBackup())
	}
	return held, stamp, nil
}

func backupMarker(root *os.Root, name string) error {
	raw, err := fsutil.ReadRootFile(root, name, 16)
	if err != nil {
		return err
	}
	if string(raw) != "1\n" {
		return invalidBackup()
	}
	return nil
}

func decodeBackup(raw []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return invalidBackup()
	}
	if _, err := d.Token(); err != io.EOF {
		return invalidBackup()
	}
	return nil
}

func readBackupHeader(line []byte) (backupHeader, error) {
	var header backupHeader
	d := json.NewDecoder(bytes.NewReader(line))
	d.DisallowUnknownFields()
	for _, expected := range []any{json.Delim('{'), "header"} {
		if token, err := d.Token(); err != nil || token != expected {
			return header, invalidBackup()
		}
	}
	if err := d.Decode(&header); err != nil || header.Schema != 1 || header.CreatedAt.IsZero() || header.FileCount < 0 || header.FileCount > 200000 {
		return header, invalidBackup()
	}
	for _, expected := range []any{"files", json.Delim('[')} {
		if token, err := d.Token(); err != nil || token != expected {
			return header, invalidBackup()
		}
	}
	if _, err := d.Token(); err != io.EOF {
		return header, invalidBackup()
	}
	return header, nil
}

func backupSummary(ctx context.Context, root *os.Root, name string) (BackupInfo, error) {
	info := BackupInfo{Name: name}
	f, err := fsutil.OpenRootRegular(root, "manifest.json")
	if err != nil {
		return info, err
	}
	r := bufio.NewReaderSize(fsutil.ContextReader(ctx, f), 64<<10)
	line, readErr := r.ReadSlice('\n')
	header, err := readBackupHeader(line)
	if err = errors.Join(readErr, err, f.Close()); err != nil {
		return info, err
	}
	info.CreatedAt, info.Files = header.CreatedAt, header.FileCount
	for _, side := range []struct {
		name    string
		version *string
	}{{"next-origin", &info.From}, {"previous-origin", &info.To}} {
		r, _, err := openBackupRoot(root, side.name)
		if err != nil {
			return info, err
		}
		header, readErr := ReadOriginHeader(ctx, r)
		err = errors.Join(readErr, r.Close())
		if err != nil {
			return info, err
		}
		*side.version = header.Pack.Version
	}
	return info, nil
}

func ReadInterruptedUpdate(ctx context.Context, content *os.Root, name string) (BackupInfo, bool, error) {
	if !backupName(name, ".tomori-pack-update-") {
		return BackupInfo{}, false, invalidBackup()
	}
	root, _, err := openBackupRoot(content, name)
	if errors.Is(err, os.ErrNotExist) {
		return BackupInfo{}, false, nil
	}
	if err != nil {
		return BackupInfo{}, false, err
	}
	defer root.Close()
	for _, marker := range []string{"committed", "recovered"} {
		err := backupMarker(root, marker)
		if err == nil {
			return BackupInfo{}, false, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return BackupInfo{}, true, err
		}
	}
	info, err := backupSummary(ctx, root, name)
	info.Interrupted = true
	return info, true, err
}

func ListBackups(ctx context.Context, content *os.Root, includeInterrupted bool) ([]BackupInfo, error) {
	if err := updateRootCurrent(content, nil); err != nil {
		return nil, err
	}
	dir, err := content.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	var result []BackupInfo
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, readErr := dir.ReadDir(128)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if !backupName(entry.Name(), ".tomori-pack-update-") || !entry.IsDir() {
				continue
			}
			count++
			if count > 512 {
				return nil, errors.New(locales.TooManyBackupsOrganizeTheBackupFolderFirst())
			}
			r, _, err := openBackupRoot(content, entry.Name())
			if err != nil {
				return nil, err
			}
			committedErr := backupMarker(r, "committed")
			interrupted := committedErr != nil
			if interrupted && !includeInterrupted {
				_ = r.Close()
				continue
			}
			if interrupted {
				recoveredErr := backupMarker(r, "recovered")
				_, manifestErr := r.Lstat("manifest.json")
				if recoveredErr == nil || errors.Is(manifestErr, os.ErrNotExist) {
					_ = r.Close()
					continue
				}
				if !errors.Is(committedErr, os.ErrNotExist) || !errors.Is(recoveredErr, os.ErrNotExist) || manifestErr != nil {
					err = errors.Join(committedErr, recoveredErr, manifestErr, r.Close())
					result = append(result, BackupInfo{Name: entry.Name(), Issue: err.Error(), Interrupted: true})
					continue
				}
			}
			_, restoredErr := r.Lstat("restored.json")
			if restoredErr == nil && !interrupted {
				_ = r.Close()
				continue
			}
			info, err := backupSummary(ctx, r, entry.Name())
			info.Interrupted = interrupted
			err = errors.Join(err, r.Close())
			if interrupted && restoredErr == nil {
				err = errors.Join(err, invalidBackup())
			}
			if restoredErr != nil && !errors.Is(restoredErr, os.ErrNotExist) {
				err = errors.Join(err, restoredErr)
			}
			if err != nil {
				info.Issue = err.Error()
			}
			result = append(result, info)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	slices.SortFunc(result, func(a, b BackupInfo) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return result, ctx.Err()
}

func readBackupRecords(ctx context.Context, root *os.Root) ([]backupRecord, error) {
	f, err := fsutil.OpenRootRegular(root, "manifest.json")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil || before.Size() > originLimit {
		return nil, errors.Join(err, invalidBackup())
	}
	r := bufio.NewReaderSize(io.LimitReader(fsutil.ContextReader(ctx, f), originLimit+1), 64<<10)
	line, err := r.ReadSlice('\n')
	if err != nil {
		return nil, errors.Join(ctx.Err(), invalidBackup())
	}
	header, err := readBackupHeader(line)
	if err != nil {
		return nil, err
	}
	var records []backupRecord
	old, targets := make(map[string]bool), make(map[string]bool)
	pathBytes := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, err = r.ReadSlice('\n')
		if err != nil {
			return nil, errors.Join(ctx.Err(), invalidBackup())
		}
		line = bytes.TrimSpace(line)
		if bytes.Equal(line, []byte("]}")) {
			break
		}
		if len(records) >= header.FileCount {
			return nil, invalidBackup()
		}
		if len(records) > 0 {
			if len(line) == 0 || line[0] != ',' {
				return nil, invalidBackup()
			}
			line = line[1:]
		}
		var record backupRecord
		if err := decodeBackup(line, &record); err != nil {
			return nil, err
		}
		if err := validateBackupRecord(record); err != nil {
			return nil, err
		}
		for _, item := range []struct {
			name string
			seen map[string]bool
		}{{record.Old, old}, {record.Target, targets}} {
			if item.name == "" {
				continue
			}
			key := strings.ToLower(item.name)
			if item.seen[key] {
				return nil, invalidBackup()
			}
			item.seen[key] = true
			pathBytes += len(item.name) + len(record.Entry.Next.Path)
		}
		if pathBytes > 16<<20 {
			return nil, invalidBackup()
		}
		record.Entry.Previous = InstalledFile{}
		record.Entry.Key, record.Entry.Path, record.Entry.Target, record.Entry.CurrentSHA1 = "", "", "", ""
		records = append(records, record)
	}
	if len(records) != header.FileCount {
		return nil, invalidBackup()
	}
	if _, err := r.ReadByte(); err != io.EOF {
		return nil, invalidBackup()
	}
	after, err := f.Stat()
	latest, latestErr := root.Lstat("manifest.json")
	if err != nil || latestErr != nil || !sameUpdateFile(before, after) || !sameUpdateFile(before, latest) {
		return nil, errors.Join(err, latestErr, invalidBackup())
	}
	return records, ctx.Err()
}

func validateBackupRecord(record backupRecord) error {
	if updateContentPath(record.Target) != nil || record.BeforeMode > 0o777 || record.AfterMode != nil && *record.AfterMode > 0o777 || record.BeforeSize < 0 || record.BeforeSize > 4<<30 {
		return invalidBackup()
	}
	if record.Old != "" {
		if updateContentPath(record.Old) != nil || !validHash(record.BeforeSHA1, 20) {
			return invalidBackup()
		}
	} else if record.BeforeSHA1 != "" || record.BeforeSize != 0 || record.BeforeMode != 0 || record.Entry.Next.Path == "" {
		return invalidBackup()
	}
	if next := record.Entry.Next; next.Path != "" {
		if updateContentPath(next.Path) != nil || next.Size < 0 || next.Size > 4<<30 || !validHash(next.SHA1, 20) {
			return invalidBackup()
		}
	} else if next != (InstalledFile{}) || record.AfterMode != nil {
		return invalidBackup()
	}
	return nil
}

func readUpdateControls(ctx context.Context, metadata, backup *os.Root, committed bool) (*os.Root, string, []backupControlRecord, error) {
	raw, err := fsutil.ReadRootFile(backup, "controls.json", 16<<10)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", nil, nil
	}
	if err != nil {
		return nil, "", nil, err
	}
	var ref struct {
		Schema      int `json:"schema"`
		Root, Stage string
	}
	if decodeBackup(raw, &ref) != nil || ref.Schema != 1 || filepath.Clean(ref.Root) != filepath.Clean(metadata.Name()) || !backupName(ref.Stage, ".tomori-pack-config-") {
		return nil, "", nil, invalidBackup()
	}
	control, _, err := openBackupRoot(metadata, ref.Stage)
	if err != nil {
		return nil, "", nil, err
	}
	failed := func(err error) (*os.Root, string, []backupControlRecord, error) {
		return nil, "", nil, errors.Join(err, control.Close())
	}
	if committed {
		if err := backupMarker(control, "ready"); err != nil {
			return failed(err)
		}
	}
	raw, err = fsutil.ReadRootFile(control, "manifest.json", 2<<20)
	if err != nil {
		return failed(err)
	}
	var manifest struct {
		Schema    int                `json:"schema"`
		CreatedAt time.Time          `json:"createdAt"`
		Parent    string             `json:"parent"`
		Files     backupControlFiles `json:"files"`
	}
	if decodeBackup(raw, &manifest) != nil || manifest.Schema != 1 || manifest.CreatedAt.IsZero() || filepath.Clean(manifest.Parent) != filepath.Clean(backup.Name()) || len(manifest.Files) == 0 || len(manifest.Files) > 64 {
		return failed(invalidBackup())
	}
	seen := make(map[string]bool)
	var total int64
	for _, file := range manifest.Files {
		key := strings.ToLower(file.Path)
		if metadataPath(file.Path) != nil || seen[key] || file.BeforeMode > 0o777 || file.AfterMode != nil && *file.AfterMode > 0o777 || file.BeforeSize < 0 || file.BeforeSize > 128<<20 || file.Old && !validHash(file.BeforeSHA1, 20) || !file.Old && (file.BeforeSHA1 != "" || file.BeforeSize != 0 || file.BeforeMode != 0) {
			return failed(invalidBackup())
		}
		seen[key] = true
		if next := file.Next; next.Path != "" {
			if next.Path != file.Path || !validHash(next.SHA1, 20) || next.Size < 0 || next.Size > 128<<20 {
				return failed(invalidBackup())
			}
		} else if !file.Old {
			return failed(invalidBackup())
		} else if next != (InstalledFile{}) || file.AfterMode != nil {
			return failed(invalidBackup())
		}
		total += file.BeforeSize + file.Next.Size
		if total > 512<<20 {
			return failed(invalidBackup())
		}
	}
	if err := ctx.Err(); err != nil {
		return failed(err)
	}
	return control, ref.Stage, manifest.Files, nil
}

func backupFileName(index int) string { return "previous/" + strconv.Itoa(index) }
