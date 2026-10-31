package minecraft

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

const (
	OfficialManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	MirrorManifestURL   = "https://bmclapi2.bangbang93.com/mc/game/version_manifest_v2.json"
)

type Manifest struct {
	Latest   LatestInfo       `json:"latest"`
	Versions []VersionSummary `json:"versions"`
}

type LatestInfo struct {
	Release  string `json:"release"`
	Snapshot string `json:"snapshot"`
}

type VersionSummary struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	URL         string    `json:"url"`
	Time        time.Time `json:"time"`
	ReleaseTime time.Time `json:"releaseTime"`
	SHA1        string    `json:"sha1"`
}

func FetchManifest(ctx context.Context, f *download.Client) (*Manifest, error) {
	url := OfficialManifestURL
	var m Manifest
	if err := f.GetJSON(ctx, url, &m); err != nil {
		return nil, fmt.Errorf(locales.VersionManifestReadFailed(), err)
	}
	if len(m.Versions) == 0 {
		return nil, errors.New(locales.VersionManifestIsEmpty())
	}
	return &m, nil
}

func FetchVersion(ctx context.Context, f *download.Client, v VersionSummary) (*VersionDetail, error) {
	if err := fsutil.ValidateID(v.ID); err != nil {
		return nil, err
	}
	var d VersionDetail
	raw, err := f.Get(ctx, v.URL)
	if err != nil {
		return nil, fmt.Errorf(locales.VersionReadFailed(), v.ID, err)
	}
	hash := sha1.Sum(raw)
	if v.SHA1 != "" && !strings.EqualFold(hex.EncodeToString(hash[:]), v.SHA1) {
		return nil, fmt.Errorf(locales.MetadataChecksumMismatchForVersion(), v.ID)
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	if d.ID == "" {
		d.ID = v.ID
	}
	if d.Type == "" {
		d.Type = v.Type
	}
	if d.ID != v.ID {
		return nil, fmt.Errorf(locales.VersionIdsMismatch(), v.ID, d.ID)
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &d, nil
}
