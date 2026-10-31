package minecraft

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/UruhaLushia/tomori/locales"
)

// Loader metadata also uses ISO-8601 offsets without a colon, unlike Mojang's
// RFC3339 timestamps. Normalize both forms when reading a launch profile.
func (v *VersionDetail) UnmarshalJSON(raw []byte) error {
	type detail VersionDetail
	var decoded detail
	envelope := struct {
		*detail
		Time          string `json:"time"`
		ReleaseTime   string `json:"releaseTime"`
		ClientVersion string `json:"clientVersion"`
	}{detail: &decoded}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	parse := func(raw string) (time.Time, error) {
		if raw == "" {
			return time.Time{}, nil
		}
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05-0700", "2006-01-02T15:04:05"} {
			if value, err := time.Parse(layout, raw); err == nil {
				return value, nil
			}
		}
		return time.Time{}, fmt.Errorf(locales.InvalidVersionTimestamp(), raw)
	}
	var err error
	decoded.Time, err = parse(envelope.Time)
	if err != nil {
		return err
	}
	decoded.ReleaseTime, err = parse(envelope.ReleaseTime)
	if err != nil {
		return err
	}
	if decoded.MinecraftVersion == "" {
		decoded.MinecraftVersion = envelope.ClientVersion
	}
	*v = VersionDetail(decoded)
	return nil
}
