package kit

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/locales"
)

type FeedbackKind string

const (
	FeedbackInfo    FeedbackKind = "info"
	FeedbackSuccess FeedbackKind = "success"
	FeedbackError   FeedbackKind = "error"
)

type StatusRow struct {
	Label, Value string
	Kind         FeedbackKind
}

type StatusData struct {
	Loading, Hint, Error string
	Rows                 []StatusRow
}

type ToastConfirmation struct {
	Deadline  time.Time
	OnConfirm func()
}

func feedbackColor(kind FeedbackKind) ui.Color {
	switch kind {
	case FeedbackError:
		return Red
	case FeedbackSuccess:
		return Green
	default:
		return Accent
	}
}

func StatusSummary(c *ui.Context, data StatusData) {
	ui.Column(c).FillWidth().Gap(5).Children(func() {
		if data.Loading != "" {
			Label(c, data.Loading, Caption)
			return
		}
		for _, item := range data.Rows {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				if item.Label != "" {
					Line(c, item.Label, Caption).Grow(1)
				}
				color := Muted
				if item.Kind != "" {
					color = feedbackColor(item.Kind)
				}
				Label(c, item.Value, Caption).TextColor(color).Shrink(0)
			})
		}
		if data.Hint != "" {
			Label(c, data.Hint, Caption).Wrap()
		}
		if data.Error != "" {
			ErrorText(c, data.Error).Wrap()
		}
	})
}

func ToastViewport(c *ui.Context, bottom float32) {
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		pad := Pad(c)
		viewport.Padding(pad, pad+12, bottom, pad+12).Gap(8).AlignItems(ui.End).Justify(ui.End)
		for i := 0; i < len(toasts); i++ {
			notice := toasts[i]
			confirmation, confirmable := notice.Data.(ToastConfirmation)
			if confirmable {
				left := confirmation.Deadline.Sub(c.Now())
				if left <= 0 {
					c.CloseToast(notice.ID)
					continue
				}
				seconds := (left + time.Second - 1) / time.Second
				notice.Description = fmt.Sprintf(locales.AutomaticallyRevertedInSeconds(), seconds)
				c.After(left - (seconds-1)*time.Second)
			}
			p := ui.ToastBase(c, notice)
			PopupMotion(c, p.Root)
			p.Root.Width(W(360)).MaxWidthPercent(92).Row().AlignItems(ui.Center).Gap(12).
				Padding(12, 12, 12, 16).Radius(RadiusL).Background(Surface).Border(1, LineColor).Shadow(0, 8, 24, 0, Shadow)
			p.Root.Children(func() {
				icon := Check
				if FeedbackKind(notice.Type) == FeedbackError {
					icon = Close
				}
				ui.Icon(c, icon).FontSize(IconPx(16)).TextColor(feedbackColor(FeedbackKind(notice.Type))).Shrink(0)
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					Line(c, notice.Title, Strong)
					if notice.Description != "" {
						Label(c, notice.Description, Caption).Wrap().MaxLines(2).Ellipsis("…")
					}
				})
				if notice.Action != "" {
					action := p.ActionButton().Height(TabH()).Shrink(0).Padding(0, 8).Radius(RadiusS).Label(notice.Action)
					if action.Hovered() || action.FocusVisible() {
						action.Background(Hover)
					}
					action.Children(func() { Label(c, notice.Action, Body).TextColor(AccentText).SingleLine() })
				}
				if confirmable {
					confirm := ui.ButtonBase(c).Label(locales.Confirm()).Height(TabH()).Padding(0, 8).Shrink(0).Radius(RadiusS).Center().OnClick(func() {
						if confirmation.OnConfirm != nil {
							confirmation.OnConfirm()
						}
						c.CloseToast(notice.ID)
					})
					if confirm.Hovered() || confirm.FocusVisible() {
						confirm.Background(Hover)
					}
					confirm.Children(func() { Label(c, locales.Confirm(), Body).SingleLine() })
				} else {
					p.CloseButton().Label(locales.Close()).Size(24, 24).Shrink(0).Center().Children(func() {
						ui.Icon(c, Close).FontSize(IconPx(12)).TextColor(Muted)
					})
				}
			})
		}
	})
}
