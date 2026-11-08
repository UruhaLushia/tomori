package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/locales"
)

type Tone int

const (
	Plain Tone = iota
	Dim
	AccentTone
	DangerTone
	SuccessTone
	WarningTone
)

func Pill(c *ui.Context, text string, tone Tone) ui.Element {
	return Badge(c, text, tone)
}

func Badge(c *ui.Context, text string, tone Tone) ui.Element {
	bg := InsetColor
	fg := Muted
	switch tone {
	case AccentTone:
		bg, fg = AccentSoft, AccentText
	case DangerTone:
		bg, fg = RedSoft, Red
	case SuccessTone:
		bg, fg = GreenSoft, Green
	case WarningTone:
		bg, fg = WarningSoft, Warning
	case Dim:
		bg, fg = InsetColor, Muted
	}
	return ui.Box(c).Height(H(20, Caption)).Shrink(0).Padding(0, 6).AlignItems(ui.Center).Radius(RadiusS).Background(bg).Children(func() {
		Label(c, text, Caption).TextColor(fg).SingleLine().Shrink(0)
	})
}

func PlayerAvatar(c *ui.Context, bitmap *ui.Bitmap, initial string, size float32) ui.Element {
	if bitmap == nil {
		return imageIcon(c, nil, nil, size, User, locales.PlayerAvatar())
	}
	return imageIcon(c, bitmap, nil, size, User, locales.PlayerAvatar())
}

func InstanceIcon(c *ui.Context, bitmap *ui.Bitmap, size float32) ui.Element {
	return imageIcon(c, bitmap, nil, size, Cube, locales.InstanceIcon())
}

func ProjectIcon(c *ui.Context, bitmap *ui.Bitmap, size float32, video ...*ui.VideoPlayer) ui.Element {
	var player *ui.VideoPlayer
	if len(video) > 0 {
		player = video[0]
	}
	return imageIcon(c, bitmap, player, size, Cube, locales.ProjectIcon())
}

func imageIcon(c *ui.Context, bitmap *ui.Bitmap, video *ui.VideoPlayer, size float32, icon *ui.SVG, label string) ui.Element {
	radius := RadiusM
	if size <= 32 {
		radius = RadiusS
	}
	if size <= 16 {
		radius = 3
	}
	box := ui.Box(c).Size(size, size).Shrink(0).Radius(radius).Clip().Background(InsetColor).Label(label)
	box.Children(func() {
		if video != nil {
			ui.Video(c, video).Fill().Fit(ui.Cover)
		} else if bitmap != nil {
			w, h := bitmap.Size()
			if max(w, h) <= 64 {
				bitmap.Pixelated()
			}
			ui.Image(c, bitmap).Fill().Fit(ui.Cover).ContentTransition(ImageMotion)
		} else {
			fontSize := float32(20)
			if size <= 32 {
				fontSize = 16
			}
			if size <= 16 {
				fontSize = 12
			}
			ui.Box(c).Fill().Center().Children(func() { ui.Icon(c, icon).FontSize(fontSize).TextColor(Muted) })
		}
		color := LineSoft
		ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) { p.Stroke(r, color, radius, 1) })
	})
	return box
}
