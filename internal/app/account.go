package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) selectAccount(id string) {
	if a.preparingGames() || a.authBusy {
		return
	}
	for _, identity := range a.cfg.Accounts.Entries {
		if identity.ID == id {
			a.cfg.Accounts.Selected = id
			a.accountError = ""
			a.saveConfig()
			return
		}
	}
}

func (a *app) removeAccount(id string) {
	if a.preparingGames() || a.authBusy {
		return
	}
	a.cfg.Accounts.Entries = slices.DeleteFunc(a.cfg.Accounts.Entries, func(identity account.Account) bool { return identity.ID == id })
	delete(a.avatars, id)
	if a.cfg.Accounts.Selected == id {
		a.cfg.Accounts.Selected = ""
	}
	a.accountError = ""
	a.saveConfig()
}

func (a *app) addOffline(name string) bool {
	if a.preparingGames() || a.authBusy {
		return false
	}
	identity, err := account.NewOffline(name)
	if err != nil {
		a.accountError = err.Error()
		return false
	}
	a.putAccount(identity, true)
	a.accountError = ""
	a.saveConfig()
	a.notify(locales.AccountAdded(), identity.Name, false)
	a.resumePendingLaunch()
	return true
}

func (a *app) putAccount(identity account.Account, selectIt bool) {
	index := slices.IndexFunc(a.cfg.Accounts.Entries, func(entry account.Account) bool { return entry.ID == identity.ID })
	if index >= 0 {
		a.cfg.Accounts.Entries[index] = identity
	} else {
		a.cfg.Accounts.Entries = append(a.cfg.Accounts.Entries, identity)
	}
	if selectIt {
		a.cfg.Accounts.Selected = identity.ID
	}
	a.loadAccountAvatar(identity)
}

func (a *app) beginAuth() (context.Context, uint64) {
	a.authID++
	ctx, cancel := context.WithCancel(a.ctx)
	a.authCancel = cancel
	a.authBusy, a.accountError, a.authStatus = true, "", locales.Connecting()
	a.authCode, a.authURL = "", ""
	return ctx, a.authID
}

func (a *app) loginMicrosoft(services ui.Services) {
	if a.preparingGames() || a.authBusy {
		return
	}
	clientID := strings.TrimSpace(account.DefaultClientID)
	proxy, budget := a.cfg.Download.Proxy, a.downloadBudget
	ctx, token := a.beginAuth()
	a.workers.Go(func() {
		client := account.NewClient(proxy, budget)
		defer client.Close()
		code, err := client.Begin(ctx, clientID)
		if err != nil {
			a.finishAuth(token, account.Account{}, err)
			return
		}
		a.win.Update(func() {
			if token == a.authID && ctx.Err() == nil {
				a.authCode, a.authURL, a.authStatus = code.UserCode, code.VerificationURI, locales.WaitingForBrowserAuthorization()
				services.WriteClipboard(code.UserCode)
				services.OpenURL(code.VerificationURI)
			}
		})
		identity, err := client.Complete(ctx, clientID, code)
		a.finishAuth(token, identity, err)
	})
}

func (a *app) finishAuth(token uint64, identity account.Account, err error) {
	if err != nil && !errors.Is(err, context.Canceled) {
		a.logf(locales.AccountAuthenticationFailed(), account.Diagnostic(err))
	} else if err == nil {
		a.logf(locales.SignedInAs(), identity.Name)
	}
	if err == nil && identity.ID != "" {
		a.queueAccountUpdate(identity, "", true)
	}
	a.win.Update(func() {
		if token != a.authID {
			return
		}
		if a.authCancel != nil {
			a.authCancel()
			a.authCancel = nil
		}
		a.authBusy, a.authCode, a.authURL, a.authStatus = false, "", "", ""
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				a.accountError = err.Error()
				a.notifyError(locales.SignInFailed(), err)
			}
			return
		}
		a.notify(locales.SignedIn(), identity.Name, false)
		a.resumePendingLaunch()
	})
}

func (a *app) cancelAuth() {
	if a.authCancel != nil {
		a.authCancel()
		// Wait for this job to return before allowing another login attempt.
		a.authStatus = locales.Canceling()
	}
}

type accountUpdate struct {
	identity account.Account
	previous string
	selectIt bool
}

// Keep completed credential rotations even if the window closes before Update.
func (a *app) queueAccountUpdate(identity account.Account, previous string, selectIt bool) {
	a.accountMu.Lock()
	a.accountUpdates = append(a.accountUpdates, accountUpdate{identity, previous, selectIt})
	a.accountMu.Unlock()
	a.win.Update(func() {
		if a.applyAccountUpdates() {
			a.saveConfig()
		}
	})
}

func (a *app) applyAccountUpdates() bool {
	a.accountMu.Lock()
	updates := a.accountUpdates
	a.accountUpdates = nil
	a.accountMu.Unlock()
	changed := false
	for _, update := range updates {
		if update.previous != "" {
			index := slices.IndexFunc(a.cfg.Accounts.Entries, func(entry account.Account) bool {
				return entry.ID == update.identity.ID && entry.SessionToken() == update.previous
			})
			if index < 0 {
				continue
			}
		}
		a.putAccount(update.identity, update.selectIt)
		changed = true
	}
	return changed
}
