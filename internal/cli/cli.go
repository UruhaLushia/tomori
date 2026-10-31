package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/egoist/mygo/i18n"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/minecraft"
)

func ListInstances(ctx context.Context, asJSON bool, out io.Writer) error {
	store, err := config.Open()
	if err != nil {
		return err
	}
	if store.Config.Appearance.Language != "" {
		i18n.SetLocaleFunc(func() string { return store.Config.Appearance.Language })
	}
	roots := make([]string, 0, len(store.Config.Directories.Entries))
	for _, entry := range store.Config.Directories.Entries {
		roots = append(roots, entry.Path)
	}
	instances, problems := minecraft.DiscoverDirectories(ctx, roots)
	if err := ctx.Err(); err != nil {
		return err
	}
	if asJSON {
		type item struct {
			Key        string                   `json:"key"`
			Name       string                   `json:"name"`
			Version    string                   `json:"version"`
			Loader     string                   `json:"loader,omitempty"`
			Format     minecraft.InstanceFormat `json:"format"`
			Path       string                   `json:"path"`
			Launchable bool                     `json:"launchable"`
		}
		items := make([]item, 0, len(instances))
		for _, instance := range instances {
			items = append(items, item{instance.Key(), instance.Name, instance.GameVersion, instance.Loader, instance.Format, instance.InstanceDir, instance.CanLaunch()})
		}
		if err := json.NewEncoder(out).Encode(items); err != nil {
			return err
		}
	} else {
		for _, instance := range instances {
			if _, err := fmt.Fprintf(out, "%s\t%s\t%s\n", strconv.Quote(instance.Key()), strconv.Quote(instance.Name), strconv.Quote(instance.GameVersion)); err != nil {
				return err
			}
		}
	}
	return errors.Join(problems...)
}
