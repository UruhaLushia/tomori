package page

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

type ResourceImportData struct {
	Open, Loading bool
	Error         string
	Files         []resource.LocalFile
	Instances     []minecraft.Instance
	Worlds        []instancecontent.File
	Target, World string
	Icons         map[string]*ui.Bitmap
}

type ResourceImportActions struct {
	Cancel         func()
	SelectInstance func(string)
	SelectWorld    func(string)
}

func ResourceImport(c *ui.Context, data *ResourceImportData, actions ResourceImportActions) {
	if !data.Open {
		return
	}
	kit.Dialog(c, &data.Open, kit.DialogM, func(_, panel ui.Element) {
		panel.MaxHeightPercent(85).Children(func() {
			kit.DialogHeader(c, locales.Import(), actions.Cancel)
			if data.Loading {
				kit.Loading(c)
				return
			}
			if data.Error != "" {
				kit.ErrorText(c, data.Error).MaxLines(4).Ellipsis("…")
				return
			}
			if len(data.Files) > 0 {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					for _, file := range data.Files {
						name := filepath.Base(file.Path)
						kit.Label(c, name+" · "+file.Kind.Label(), kit.Caption).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
					}
				})
			}
			if data.Target == "" {
				kit.Label(c, locales.ChooseTargetInstance(), kit.Body)
				if len(data.Instances) == 0 {
					kit.EmptyState(c, kit.Cube, locales.NoAvailableInstances(), locales.CreateAnInstanceOrImportAModpackToGetStarted(), nil)
					return
				}
				ui.Scroll(c).Scrollbars(false).FillWidth().Grow(1).MinHeight(0).Children(func() {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						for _, instance := range data.Instances {
							row := kit.RowButton(c, "", false, func() { actions.SelectInstance(instance.Key()) }).Padding(10, 12).Gap(10)
							row.Children(func() {
								kit.InstanceIcon(c, data.Icons[instance.Key()], 32)
								ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
									kit.Label(c, instance.Name, kit.Strong).MaxLines(1).Ellipsis("…")
									kit.Label(c, kit.MetaLine(instance.GameVersion, instance.Loader), kit.Caption).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
								})
							})
						}
					})
				})
				return
			}
			if len(data.Worlds) > 0 && data.World == "" {
				kit.Label(c, locales.ChooseWorldFolder(), kit.Body)
				ui.Scroll(c).Scrollbars(false).FillWidth().Grow(1).MinHeight(0).Children(func() {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						for _, world := range data.Worlds {
							row := kit.RowButton(c, world.Name, false, func() { actions.SelectWorld(world.Path) }).Padding(10, 12)
							row.Children(func() { kit.Label(c, world.Name, kit.Strong).MaxLines(1).Ellipsis("…") })
						}
					})
				})
			}
		})
	})
}
