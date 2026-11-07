package app

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) notify(title, description string, failed bool) {
	if a.ctx.Err() != nil {
		return
	}
	t := ui.Toast{ID: title + "\x00" + description, Title: title, Description: description, Type: string(kit.FeedbackSuccess), Timeout: 5 * time.Second}
	if failed {
		t.Type, t.Timeout = string(kit.FeedbackError), 7*time.Second
		t.Action, t.OnAction = locales.ViewLogs(), func() { a.goPage(pageLogs) } // nav:toast
	}
	a.enqueueNotice(t)
}

func (a *app) notifyInstalled(source, name, key string) {
	if source == "" {
		source = key
	}
	a.enqueueNotice(ui.Toast{ID: source, Title: name + " " + locales.Installed3(), Type: string(kit.FeedbackSuccess), Timeout: 5 * time.Second, Action: locales.View(), OnAction: func() {
		a.goPage(pageInstances) // nav:toast
		a.pages.instances.Open(key)
	}})
}

func (a *app) enqueueNotice(t ui.Toast) {
	for i := range a.notices {
		if a.notices[i].ID == t.ID {
			a.notices[i] = t
			return
		}
	}
	if len(a.notices) == 3 {
		copy(a.notices, a.notices[1:])
		a.notices = a.notices[:2]
	}
	a.notices = append(a.notices, t)
}

func (a *app) notifyError(title string, err error) {
	if err != nil && !operationCanceled(err) {
		a.notify(title, failureReason(err), true)
	}
}

func operationCanceled(err error) bool {
	if err == context.Canceled {
		return true
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		children := wrapped.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !operationCanceled(child) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		return operationCanceled(wrapped.Unwrap())
	default:
		return false
	}
}

func failureReason(err error) string {
	var connection net.Error
	var exit *exec.ExitError
	if command, ok := errors.AsType[*launcher.CommandError](err); ok {
		if errors.Is(command.Err, context.DeadlineExceeded) {
			return locales.CommandTimedOut()
		}
		if command.Phase() == locales.PostExitCommand() {
			return locales.PostExitCommandFailed()
		}
		return locales.PreLaunchCommandFailed()
	}
	switch {
	case storage.IsFull(err):
		return locales.NotEnoughDiskSpace()
	case errors.Is(err, os.ErrPermission):
		return locales.FileAccessDenied()
	case errors.Is(err, os.ErrNotExist):
		return locales.ARequiredFileIsMissing()
	case errors.Is(err, context.DeadlineExceeded):
		return locales.RequestTimedOutCheckYourConnection()
	case errors.As(err, &connection):
		return locales.NetworkConnectionFailed()
	case errors.As(err, &exit):
		return locales.TheProcessExitedUnexpectedlySeeLogsForDetails()
	default:
		message := err.Error()
		for _, marker := range []string{"账户认证失败", "Account authentication failed"} {
			if strings.Contains(message, marker) {
				return locales.AuthenticationFailedSignInAgain()
			}
		}
		if strings.Contains(message, "Java") {
			return locales.JavaIsUnavailableOrIncompatible()
		}
		return locales.TheOperationFailedSeeLogsForDetails()
	}
}

func (a *app) notificationsView(c *ui.Context) {
	for _, notice := range a.notices {
		c.AddToast(notice)
	}
	clear(a.notices)
	a.notices = a.notices[:0]
	bottom := kit.Pad(c) + 12
	if a.page == pageHome {
		bottom += kit.H(64, kit.Strong, kit.Caption)
	}
	kit.ToastViewport(c, bottom)
}
