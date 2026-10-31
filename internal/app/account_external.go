package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/locales"
)

type externalAuthState struct {
	active   bool
	server   account.YggdrasilServer
	session  account.YggdrasilSession
	expected account.Account
}

func (a *app) beginExternalAuth(id string) {
	if a.preparingGames() || a.authBusy {
		return
	}
	a.externalAuth = externalAuthState{active: true}
	a.accountError = ""
	for _, entry := range a.cfg.Accounts.Entries {
		if entry.ID == id && entry.Kind == account.Yggdrasil {
			a.externalAuth.expected = entry
			break
		}
	}
}

func (a *app) closeExternalAuth() {
	if a.externalAuth.active {
		a.cancelAuth()
	}
	a.externalAuth = externalAuthState{}
	a.dialogs.accounts.CloseExternal()
}

func (a *app) inspectExternalServer(address string) {
	if a.preparingGames() || a.authBusy || !a.externalAuth.active {
		return
	}
	discover := a.externalAuth.expected.ID == ""
	if !discover {
		address = a.externalAuth.expected.ServerURL
	}
	ctx, token := a.beginAuth()
	proxy, budget := a.cfg.Download.Proxy, a.downloadBudget
	a.workers.Go(func() {
		client := account.NewClient(proxy, budget)
		defer client.Close()
		server, err := client.YggdrasilServer(ctx, address, discover)
		a.completeExternalAuth(ctx, token, err, func() {
			a.externalAuth.server = server
			a.saveAuthServer(server.Name, server.URL)
		})
	})
}

func (a *app) saveAuthServer(name, address string) bool {
	if a.preparingGames() || a.authBusy || a.pathBusy {
		return false
	}
	address, err := account.NormalizeYggdrasilURL(address)
	if err != nil {
		a.accountError = err.Error()
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = address
	}
	if len([]rune(name)) > 128 {
		name = string([]rune(name)[:128])
	}
	server := config.AuthServer{Name: name, URL: address}
	index := slices.IndexFunc(a.cfg.Accounts.Servers, func(s config.AuthServer) bool { return s.URL == address })
	if index < 0 {
		a.cfg.Accounts.Servers = append(a.cfg.Accounts.Servers, server)
	} else {
		a.cfg.Accounts.Servers[index] = server
	}
	a.accountError = ""
	a.saveConfig()
	return true
}

func (a *app) removeAuthServer(address string) {
	if a.preparingGames() || a.authBusy || a.pathBusy {
		return
	}
	for _, identity := range a.cfg.Accounts.Entries {
		if identity.ServerURL == address {
			return
		}
	}
	a.cfg.Accounts.Servers = slices.DeleteFunc(a.cfg.Accounts.Servers, func(s config.AuthServer) bool { return s.URL == address })
	a.saveConfig()
}

func (a *app) loginExternal(login, password string) {
	if a.preparingGames() || a.authBusy || !a.externalAuth.active || a.externalAuth.server.Metadata == "" {
		return
	}
	server, expected := a.externalAuth.server, a.externalAuth.expected
	if expected.ID != "" {
		login = expected.LoginName
	}
	ctx, token := a.beginAuth()
	proxy, budget := a.cfg.Download.Proxy, a.downloadBudget
	a.workers.Go(func() {
		client := account.NewClient(proxy, budget)
		defer client.Close()
		session, err := client.LoginYggdrasil(ctx, server, login, password)
		if err != nil {
			a.completeExternalAuth(ctx, token, err, nil)
			return
		}
		if session.Selected != nil || expected.ID != "" {
			identity, err := client.SelectYggdrasil(ctx, session, expected.UUID)
			a.completeExternalAuth(ctx, token, err, func() { a.acceptExternalAccount(identity) })
			return
		}
		a.completeExternalAuth(ctx, token, nil, func() { a.externalAuth.session = session })
	})
}

func (a *app) selectExternalProfile(profile string) {
	if a.preparingGames() || a.authBusy || !a.externalAuth.active || len(a.externalAuth.session.Profiles) == 0 {
		return
	}
	session := a.externalAuth.session
	ctx, token := a.beginAuth()
	proxy, budget := a.cfg.Download.Proxy, a.downloadBudget
	a.workers.Go(func() {
		client := account.NewClient(proxy, budget)
		defer client.Close()
		identity, err := client.SelectYggdrasil(ctx, session, profile)
		a.completeExternalAuth(ctx, token, err, func() { a.acceptExternalAccount(identity) })
	})
}

func (a *app) acceptExternalAccount(identity account.Account) {
	a.putAccount(identity, true)
	a.saveConfig()
	a.externalAuth = externalAuthState{}
	a.notify(locales.SignedIn(), identity.Name, false)
	a.resumePendingLaunch()
}

func (a *app) completeExternalAuth(ctx context.Context, token uint64, err error, apply func()) {
	a.win.Update(func() {
		if a.authID != token {
			return
		}
		canceled := errors.Is(ctx.Err(), context.Canceled)
		if a.authCancel != nil {
			a.authCancel()
			a.authCancel = nil
		}
		a.authBusy, a.authCode, a.authURL, a.authStatus = false, "", "", ""
		if a.ctx.Err() != nil || !a.externalAuth.active || canceled || errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			a.accountError = err.Error()
			a.logf(locales.AccountAuthenticationFailed(), account.Diagnostic(err))
			a.notifyError(locales.SignInFailed(), err)
			return
		}
		apply()
	})
}

func (a *app) backExternalAuth() {
	if a.authBusy {
		return
	}
	a.accountError = ""
	if len(a.externalAuth.session.Profiles) > 0 {
		a.externalAuth.session = account.YggdrasilSession{}
	} else {
		a.externalAuth.server = account.YggdrasilServer{}
	}
}
