package app

import (
	"cmp"
	"slices"

	"github.com/UruhaLushia/tomori/internal/minecraft"
)

// Unknown instances keep discovery order after those the user arranged.
func orderInstances(instances []minecraft.Instance, order []string) {
	if len(instances) < 2 || len(order) == 0 {
		return
	}
	rank := make(map[string]int, len(order))
	for i, key := range order {
		if _, exists := rank[key]; !exists {
			rank[key] = i
		}
	}
	position := func(key string) int {
		if i, ok := rank[key]; ok {
			return i
		}
		return len(order)
	}
	slices.SortStableFunc(instances, func(a, b minecraft.Instance) int {
		return cmp.Compare(position(a.Key()), position(b.Key()))
	})
}

// before is an instance key, or empty to move to the end. Keys also keep
// drops correct when the grid is filtered or discovery finishes mid-drag.
func (a *app) reorderInstance(key, before string) {
	if key == before {
		return
	}
	from := slices.IndexFunc(a.installedList, func(v minecraft.Instance) bool { return v.Key() == key })
	if from < 0 {
		return
	}
	list := slices.Clone(a.installedList)
	instance := list[from]
	list = slices.Delete(list, from, from+1)
	to := len(list)
	if before != "" {
		to = slices.IndexFunc(list, func(v minecraft.Instance) bool { return v.Key() == before })
		if to < 0 {
			return
		}
	}
	if to == from {
		return
	}
	a.installedList = slices.Insert(list, to, instance)
	order := make([]string, len(a.installedList))
	for i, v := range a.installedList {
		order[i] = v.Key()
	}
	a.cfg.Directories.InstanceOrder = order
	a.saveConfig()
}
