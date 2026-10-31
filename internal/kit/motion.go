package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
)

type motionStateKey string
type groupMotionKey string
type settingMotionKey string

const (
	motionFast    = 120 * time.Millisecond
	motionMove    = 220 * time.Millisecond
	motionPageTop = 300 * time.Millisecond
	motionPage    = 320 * time.Millisecond
	motionNavRail = 240 * time.Millisecond
	motionUnfold  = 300 * time.Millisecond
	motionShow    = 480 * time.Millisecond
)

var (
	MotionFast        = ui.Animation{Duration: motionFast, Ease: ui.EaseOut}
	MotionMove        = ui.Animation{Duration: motionMove, Ease: ui.EaseOut}
	MotionPageTop     = ui.Animation{Duration: motionPageTop, Ease: ui.EaseInOut}
	MotionPage        = ui.Animation{Duration: motionPage, Ease: ui.EaseInOut}
	MotionNavRail     = ui.Animation{Duration: motionNavRail, Ease: ui.EaseOut}
	MotionInOut       = ui.Animation{Duration: motionMove, Ease: ui.EaseInOut}
	MotionUnfold      = ui.Animation{Duration: motionUnfold, Ease: ui.EaseOut}
	MotionShow        = ui.Animation{Duration: motionShow, Ease: ui.EaseOut}
	MotionSpring      = ui.Animation{Spring: ui.SpringOptions{Duration: motionMove, Bounce: 0.06}}
	ImageMotion       = ui.ContentTransition{Animation: &MotionFast}
	ValueMotion       = ui.ContentTransition{Animation: &MotionMove}
	NumberMotion      = ui.ContentTransition{Numeric: true, Animation: &MotionMove}
	layoutTransition  = ui.ElementTransition{Position: true, Animation: &MotionMove}
	contentTransition = ui.ElementTransition{
		Position: true, Animation: &MotionMove, Enter: &ui.Motion{Y: 4, Opacity: 0}, Exit: &ui.Motion{Y: -2, Opacity: 0},
	}
	presenceTransition = ui.ElementTransition{
		Position: true, Animation: &MotionFast,
		Enter: &ui.Motion{Y: 3}, Exit: &ui.Motion{Y: -2, Opacity: 0},
	}
	foldTransition = ui.ElementTransition{
		Position: true, Animation: &MotionUnfold,
		Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true},
	}
	popupTransition = ui.ElementTransition{
		Position: true, Animation: &MotionFast,
		Enter: &ui.Motion{Y: -4, Scale: 0.985}, Exit: &ui.Motion{Y: -2, Scale: 0.99},
	}
)

func NavigationPageMotion(el ui.Element, horizontal bool, direction int) ui.Element {
	if direction == 0 {
		direction = 1
	}
	enter := &ui.Motion{Opacity: 0.35}
	if horizontal {
		enter.X = float32(direction * 18)
	} else {
		enter.Y = float32(direction * 14)
	}
	animation := &MotionPage
	if horizontal {
		animation = &MotionPageTop
	}
	return el.Transition(ui.ElementTransition{Position: true, Colors: true, Animation: animation, Enter: enter})
}

func PageMotion(c *ui.Context, el ui.Element) ui.Element {
	return NavigationPageMotion(el, false, 1)
}

func ContentMotion(c *ui.Context, el ui.Element) ui.Element {
	return el.Transition(contentTransition)
}

// SlideMotion uses opposite entry directions for forward and back navigation.
func SlideMotion(el ui.Element, forward bool) ui.Element {
	x := float32(12)
	if !forward {
		x = -x
	}
	return el.Transition(ui.ElementTransition{
		Position: true, Animation: &MotionMove, Enter: &ui.Motion{X: x, Opacity: 0.15},
	})
}

func Crossfade(el ui.Element) ui.Element {
	return el.Transition(ui.ElementTransition{Animation: &MotionFast, Enter: &ui.Motion{}, Exit: &ui.Motion{}})
}

func LayoutMotion(el ui.Element) ui.Element {
	return el.Transition(layoutTransition)
}

func PresenceMotion(el ui.Element) ui.Element {
	return el.Transition(presenceTransition)
}

func FoldMotion(el ui.Element) ui.Element {
	return el.Transition(foldTransition)
}

func MatchedMotion(el ui.Element, key any) ui.Element {
	transition := presenceTransition
	transition.Match = key
	return el.Transition(transition)
}

func ControlMotion(el ui.Element) ui.Element {
	pose := ui.Pose{}
	if el.Pressed() && !el.IsDisabled() {
		pose.Scale = 0.98
	}
	return LayoutMotion(el).AnimatePose(pose, MotionFast)
}

func Progress(c *ui.Context, value float64) ui.Element {
	if value < 0 {
		return ui.Progress(c, value)
	}
	target := float32(max(0, min(1, value)))
	el := ui.Box(c).Height(4).Radius(RadiusFull).Background(LineSoft).Clip().Role(ui.RoleProgress).Range(0, 1, float64(target))
	state := ui.Local(el, "progress", func() progressMotion { return progressMotion{from: target, to: target, at: c.Now()} })
	if state.to != target {
		state.from, state.to, state.at = state.value(c.Now()), target, c.Now()
	}
	if c.Preferences().ReduceMotion {
		state.from = target
	}
	color := Accent
	return el.Draw(func(p *ui.Painter, r ui.Rect) {
		fill := state.value(p.Now())
		if state.from != state.to && p.Now().Sub(state.at) < motionFast {
			p.AnimationFrame()
		}
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * fill, H: r.H}, color, r.H/2)
	})
}

type progressMotion struct {
	from, to float32
	at       time.Time
}

func (p *progressMotion) value(now time.Time) float32 {
	t := min(float32(1), max(float32(0), float32(now.Sub(p.at))/float32(motionFast)))
	return p.from + (p.to-p.from)*ui.EaseOut(t)
}

func CardMotion(el ui.Element) ui.Element {
	pose := ui.Pose{}
	if el.Pressed() {
		pose.Scale = 0.985
	} else if el.Hovered() {
		pose.Y = -2
	}
	return el.AnimatePose(pose, MotionSpring)
}

func PopupMotion(c *ui.Context, el ui.Element) ui.Element {
	return el.TransformOrigin(0.5, 0).Transition(popupTransition)
}

type DialogSize int

const (
	DialogS      DialogSize = 440
	DialogM      DialogSize = 560
	DialogL      DialogSize = 720
	DialogViewer DialogSize = -1
)

func Dialog(c *ui.Context, open *bool, size DialogSize, build func(back, panel ui.Element)) ui.Element {
	return ui.DialogBase(c, open, func(back, panel ui.Element) {
		back.Background(Backdrop)
		width, height := c.Size()
		panel.Width(min(W(float32(size)), width-48)).MaxHeight(min(640, height*.9)).Padding(24).Gap(16).Radius(RadiusL).
			Background(Surface).Border(1, LineColor).Shadow(0, 8, 24, 0, Shadow)
		if size == DialogViewer {
			panel.WidthPercent(90).MaxHeightPercent(90)
		}
		back.Transition(ui.ElementTransition{
			Animation: &MotionMove,
			Colors:    true,
			Enter:     &ui.Motion{},
			Exit:      &ui.Motion{},
		})
		// DialogBase creates the backdrop and panel together. Seed the panel's
		// own motion so its entry still plays inside that new backdrop.
		entered := ui.Local(panel, "dialog-entered", func() bool { return false })
		if !*entered {
			panel.Jump("dialog-entry", 0)
			*entered = true
		}
		progress := panel.AnimateTo("dialog-entry", 1, LayoutAnimation(MotionMove))
		panel.Translate(0, 10*(1-progress)).Transition(ui.ElementTransition{
			Position: true, Size: true, Animation: &MotionMove,
		})
		build(back, panel)
	})
}
