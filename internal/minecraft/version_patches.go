package minecraft

import (
	"cmp"
	"encoding/json"
	"math"
	"slices"
	"strings"
)

// VersionPatch uses the same launch metadata as a version, with HMCL/SJMCL's
// component version and priority. Keep a single decoder for library and time
// formats across standalone, inherited and patched profiles.
type VersionPatch struct {
	VersionDetail
	Version  string `json:"version,omitempty"`
	Priority *int64 `json:"priority,omitempty"`
}

func (p *VersionPatch) UnmarshalJSON(raw []byte) error {
	var metadata struct {
		Version  string `json:"version"`
		Priority *int64 `json:"priority"`
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return err
	}
	var detail VersionDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return err
	}
	*p = VersionPatch{VersionDetail: detail, Version: metadata.Version, Priority: metadata.Priority}
	return nil
}

func applyVersionPatches(base *VersionDetail, patches []VersionPatch) *VersionDetail {
	ordered := slices.Clone(patches)
	priority := func(p VersionPatch) int64 {
		if p.Priority != nil {
			return *p.Priority
		}
		return math.MinInt64
	}
	slices.SortStableFunc(ordered, func(a, b VersionPatch) int { return cmp.Compare(priority(a), priority(b)) })
	hasForge := slices.ContainsFunc(ordered, func(p VersionPatch) bool {
		return p.ID == "forge" || p.ID == "legacyforge" || p.ID == "cleanroom"
	})
	var optifineArgs []Argument
	for _, patch := range ordered {
		detail := patch.VersionDetail
		detail.ID, detail.Jar, detail.InheritsFrom, detail.Patches = "", "", "", nil
		if patch.ID == "game" && patch.Version != "" {
			detail.MinecraftVersion = patch.Version
		}
		if patch.ID == "optifine" {
			// OptiFine's legacy flags are incremental; replacing the vanilla
			// template would lose authentication and game directory arguments.
			detail.Arguments.Game = slices.Concat(legacyArguments(detail.MinecraftArguments), detail.Arguments.Game)
			detail.MinecraftArguments = ""
			from, to := "optifine.OptiFineForgeTweaker", "optifine.OptiFineTweaker"
			if hasForge {
				from, to = to, from
			}
			for n, arg := range detail.Arguments.Game {
				detail.Arguments.Game[n].Value = slices.Clone(arg.Value)
				for i, value := range arg.Value {
					detail.Arguments.Game[n].Value[i] = strings.ReplaceAll(value, from, to)
				}
			}
			// Forge's legacy template has higher priority than OptiFine and
			// replaces the full argument string. Add incremental flags after
			// that replacement so its tweak class survives either format.
			optifineArgs = append(optifineArgs, detail.Arguments.Game...)
			detail.Arguments.Game = nil
		}
		base = MergeVersion(base, &detail)
	}
	if len(optifineArgs) > 0 {
		base = MergeVersion(base, &VersionDetail{Arguments: Arguments{Game: optifineArgs}})
	}
	return base
}
