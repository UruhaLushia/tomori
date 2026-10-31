package launcher

import (
	"encoding/json"
	"errors"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

type exportMCBBSFiles struct {
	files []json.RawMessage
	bytes int
}

func (f *exportMCBBSFiles) add(path, hash string) error {
	raw, err := json.Marshal(struct {
		Type  string `json:"type"`
		Force bool   `json:"force"`
		Path  string `json:"path"`
		Hash  string `json:"hash"`
	}{"addon", true, path, hash})
	if err != nil {
		return err
	}
	if len(raw)+1 > (16<<20)-(64<<10)-f.bytes {
		return errors.New(locales.ExportFileCountOrSizeExceedsTheLimit())
	}
	f.bytes += len(raw) + 1
	f.files = append(f.files, raw)
	return nil
}

func writeExportMCBBS(z *fsutil.ZipExporter, request PackExport, components minecraft.InstanceComponents, files []json.RawMessage) error {
	type addon struct {
		ID      string `json:"id"`
		Version string `json:"version"`
	}
	manifest := struct {
		Type        string            `json:"manifestType"`
		Schema      int               `json:"manifestVersion"`
		Name        string            `json:"name"`
		Version     string            `json:"version"`
		Author      string            `json:"author"`
		Description string            `json:"description"`
		Addons      []addon           `json:"addons"`
		Files       []json.RawMessage `json:"files"`
	}{"minecraftModpack", 2, request.Name, request.Version, "", request.Description, []addon{{"game", components.Game}}, files}
	if manifest.Files == nil {
		manifest.Files = []json.RawMessage{}
	}
	if components.Loader != "" {
		manifest.Addons = append(manifest.Addons, addon{components.Loader, components.LoaderVersion})
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := z.Bytes("mcbbs.packmeta", raw); err != nil {
		return err
	}
	return writeExportCurse(z, request, components, nil)
}
