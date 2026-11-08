package page

import (
	"fmt"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo/ui"
	"path/filepath"
	"strings"
)

type PackImportData struct {
	Space                                     SpaceData
	Open, Loading, Busy, Optional, Deferred   bool
	Step                                      int
	Entry                                     string
	Path, Name, Description, Directory, Error string
	Mode, URL, LoadingLabel, IconURL          string
	Icon                                      *ui.Bitmap
	Info                                      modpack.Info
	Directories                               []config.GameDirectory
	LocalCount                                int
	Missing                                   []modpack.MissingFile
	SelectedOptional                          map[string]bool
	Estimate                                  int64
}
type PackImportActions struct {
	Close, Browse, Import, Supplement func()
	FetchURL, Online                  func()
	ChangeSource                      func(string)
	Drop                              func([]string)
}

func PackImport(c *ui.Context, data *PackImportData, actions PackImportActions) {
	if !data.Open {
		return
	}
	if data.Step == 0 {
		data.Step = 1
	}
	if data.Entry == "" {
		data.Entry = "import"
	}
	if data.Mode == "" {
		data.Mode = "local"
	}
	c.Key("pack-import")
	dialog := kit.Dialog(c, &data.Open, kit.DialogM, func(back, panel ui.Element) {
		if !data.Busy && actions.Drop != nil {
			if files := panel.DroppedFiles(); len(files) > 0 {
				actions.Drop(files)
			}
		}
		panel.Children(func() {
			title := locales.ImportModpack()
			if data.Entry == "detail" {
				title = locales.InstallModpack()
			}
			var back func()
			if data.Entry == "import" && data.Step == 2 {
				back = func() { kit.BackButton(c, locales.ChooseSource(), func() { actions.ChangeSource(data.Mode) }) }
			}
			kit.DialogHeader(c, title, actions.Close, back)
			step := fmt.Sprintf("pack-step:%d:%s", data.Step, data.Mode)
			kit.SlideMotion(ui.Column(c.Key(step)).FillWidth().MinHeight(0).Shrink(1).Gap(16), data.Step > 1).Children(func() {
				if data.Step == 1 {
					packImportSource(c, data, actions, panel.FileDragOver())
				} else {
					if data.Loading {
						ui.Row(c).FillWidth().Height(kit.H(80, kit.Body)).Gap(8).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
							c.Key("pack-reading")
							ui.Spinner(c).Size(16, 16)
							kit.Label(c, locales.ReadingModpack2(), kit.BodyMuted)
						})
					} else if data.Info.Format != "" {
						packInstallForm(c, data, actions)
					} else if data.Error != "" {
						kit.ErrorText(c, data.Error).Wrap()
					}
				}
			})
			ui.Row(c).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
				if data.Entry == "import" {
					kit.Line(c, fmt.Sprintf(locales.StepOf2(), data.Step), kit.Caption).ContentTransition(kit.NumberMotion)
				} else if !data.Loading {
					kit.Line(c, locales.EstimatedDownload()+" "+transferSize(packEstimate(data)), kit.Caption)
				}
				ui.Spacer(c)
				kit.Ghost(c, locales.Cancel(), actions.Close)
				if data.Step == 1 && data.Mode == "url" {
					kit.Primary(c, locales.ReadingModpack(), actions.FetchURL).Disabled(data.Busy || strings.TrimSpace(data.URL) == "")
				}
				if data.Step == 2 && !data.Loading && data.Info.Format != "" {
					kit.Primary(c, locales.Install(), actions.Import).Disabled(data.Busy || data.Space.Blocked() || strings.TrimSpace(data.Name) == "" || data.Directory == "" || len(data.Info.Warnings) > 0)
				}
			})
		})
	})
	if dialog.Dismissed() {
		actions.Close()
	}
}
func packImportSource(c *ui.Context, data *PackImportData, actions PackImportActions, drag bool) {
	kit.SegmentedValues(c, &data.Mode, []kit.SelectOption[string]{{Value: "local", Label: locales.LocalFile()}, {Value: "url", Label: locales.Link2()}}, func() { actions.ChangeSource(data.Mode) }).AlignSelf(ui.Start)
	if data.Mode == "url" {
		kit.Field(c, locales.ModpackLink(), func() {
			c.Key("pack-url")
			kit.Input(c, &data.URL).FillWidth().Placeholder(locales.DownloadLinkOrProjectPage()).Label(locales.ModpackUrl()).OnSubmit(actions.FetchURL).Disabled(data.Busy)
		})
		kit.Label(c, locales.AModrinthOrCurseForgeProjectOrADirectZipMrpackLink(), kit.Caption).Wrap()
		if data.Error != "" {
			kit.ErrorText(c, data.Error).Wrap()
		}
		return
	}
	bg, icon := kit.InsetColor, kit.Muted
	if drag {
		bg = kit.AccentSoft
		icon = kit.Accent
	} else if data.Error != "" {
		bg = kit.RedSoft
		icon = kit.Red
	}
	ui.Column(c).FillWidth().Height(kit.H(160, kit.Strong, kit.Caption, kit.Body)).Radius(kit.RadiusM).Background(bg).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		kit.IconBadge(c, kit.Download, 40, icon).Background(kit.Surface)
		kit.Label(c, locales.DropAModpackFileHere(), kit.Strong).Margin(12, 0, 4, 0)
		kit.Label(c, locales.ZipMrpackModrinthCurseForgeMultiMc(), kit.Caption).MaxLines(2).Wrap().TextAlign(ui.Center)
		kit.Button(c, locales.ChooseFile(), actions.Browse).Margin(16, 0, 0, 0).Disabled(data.Busy)
	})
	if data.Error != "" {
		kit.ErrorText(c, data.Error).Wrap()
	}
}
func packInstallForm(c *ui.Context, data *PackImportData, actions PackImportActions) {
	services := c.Services()
	ui.Scroll(c).Scrollbars(false).FillWidth().MinHeight(0).Shrink(1).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Gap(16).Children(func() {
			ui.Row(c).FillWidth().MinHeight(kit.H(44, kit.Strong, kit.Caption)).Gap(12).AlignItems(ui.Center).Children(func() {
				kit.ProjectIcon(c, data.Icon, 44)
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					kit.Line(c, kit.MetaLine(data.Info.Name, data.Info.Version), kit.Strong)
					kit.Line(c, kit.MetaLine(data.Info.GameVersion, data.Info.LoaderLabel(), fmt.Sprintf(locales.Mods2(), len(data.Info.Files))), kit.Caption)
				})
			})
			kit.Field(c, locales.InstanceName(), func() {
				c.Key("pack-name")
				kit.Input(c, &data.Name).FillWidth().Label(locales.InstanceName()).Disabled(data.Busy)
			})
			kit.Field(c, locales.InstallLocation2(), func() {
				options := make([]kit.SelectOption[string], 0, len(data.Directories))
				label := data.Directory
				for _, directory := range data.Directories {
					options = append(options, kit.SelectOption[string]{Value: directory.Path, Label: directory.Name})
					if directory.Path == data.Directory {
						label = directory.Name
					}
				}
				c.Key("pack-directory")
				kit.Select(c, &data.Directory, locales.GameDirectories()+" · "+label, options, 200, 320, nil).FillWidth().Label(locales.InstallLocation2()).Disabled(data.Busy)
			})
			var optional []modpack.File
			for _, file := range data.Info.Files {
				if file.Optional {
					optional = append(optional, file)
				}
			}
			if len(optional) > 0 {
				if data.SelectedOptional == nil {
					data.SelectedOptional = make(map[string]bool)
				}
				kit.FormGroup(c, locales.OptionalFiles(), func() { kit.Label(c, fmt.Sprintf(locales.Items3(), len(optional)), kit.Caption) }, func() {
					for i, file := range optional {
						if i > 0 {
							kit.Divider(c)
						}
						key := file.SelectionKey()
						selected := data.SelectedOptional[key]
						name := filepath.Base(file.Path)
						if file.Path == "" {
							name = fmt.Sprintf("CurseForge · %d / %d", file.ProjectID, file.FileID)
						}
						kit.FormRow(c, name, "", func() {
							kit.Switch(c, &selected).Label(name).OnClick(func() { data.SelectedOptional[key] = !data.SelectedOptional[key] })
						})
					}
				})
			} else if data.Deferred {
				kit.FormRow(c, locales.InstallOptionalFiles2(), "", func() { kit.Switch(c, &data.Optional) })
			}
			if data.Error != "" {
				kit.ErrorText(c, data.Error).Wrap()
			}
			if len(data.Info.Warnings) > 0 {
				kit.ErrorText(c, strings.Join(data.Info.Warnings, "；")).Wrap()
			}
			if len(data.Missing) > 0 {
				kit.FormGroup(c, locales.AddMissingFiles(), nil, func() {
					for _, file := range data.Missing {
						kit.FormRow(c, file.Name, "", func() {
							if file.Website != "" {
								kit.Ghost(c, locales.DownloadsPage(), func() { services.OpenURL(file.Website) })
							}
						})
					}
					kit.Button(c, locales.ChooseDownloadedFile(), actions.Supplement).Disabled(data.Busy)
				})
			}
			if data.LocalCount > 0 {
				kit.Label(c, fmt.Sprintf(locales.AddedFiles(), data.LocalCount), kit.Caption)
			}
			if data.Entry == "import" {
				kit.Label(c, locales.EstimatedDownload()+" "+transferSize(packEstimate(data)), kit.Caption)
			}
			if data.Space.Report.Check() != nil {
				kit.ErrorText(c, locales.NotEnoughDiskSpace())
			}
		})
	})
}
func packEstimate(data *PackImportData) int64 {
	if data.Deferred {
		return data.Estimate
	}
	var total int64
	for _, file := range data.Info.Files {
		if !file.Optional || data.SelectedOptional[file.SelectionKey()] {
			total += max(0, file.Size)
		}
	}
	return total
}
