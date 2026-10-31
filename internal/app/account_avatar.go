package app

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/locales"
)

type accountAvatar struct {
	bitmap  *ui.Bitmap
	skinURL string
	token   uint64
	session string
}

func (a *app) loadAccountAvatar(identity account.Account) {
	if a.ctx.Err() != nil {
		return
	}
	if entry, ok := a.avatars[identity.ID]; ok && entry.skinURL == identity.SkinURL && (identity.Kind != account.Yggdrasil || entry.session == identity.AccessToken) {
		return
	}
	if a.avatars == nil {
		a.avatars = make(map[string]accountAvatar)
	}
	a.avatarID++
	token := a.avatarID
	session := ""
	if identity.Kind == account.Yggdrasil {
		session = identity.AccessToken
	}
	a.avatars[identity.ID] = accountAvatar{bitmap: ui.NewBitmap(account.DefaultAvatar(identity)), skinURL: identity.SkinURL, token: token, session: session}
	if identity.Kind != account.Microsoft && identity.Kind != account.Yggdrasil {
		return
	}
	cacheDir := a.cfg.CacheDirectory()
	proxy := a.cfg.Download.Proxy
	budget := a.downloadBudget
	a.workers.Go(func() {
		client := account.NewClient(proxy, budget)
		defer client.Close()
		portrait, err := client.Avatar(a.ctx, cacheDir, identity)
		var bitmap *ui.Bitmap
		if portrait != nil {
			bitmap = ui.NewBitmap(portrait)
		}
		a.win.Update(func() {
			entry, ok := a.avatars[identity.ID]
			if a.ctx.Err() != nil || !ok || entry.token != token {
				return
			}
			if bitmap != nil {
				entry.bitmap = bitmap
				a.avatars[identity.ID] = entry
			}
			if err != nil {
				a.logf(locales.FailedToLoadAvatarFor(), identity.Name, err)
			}
		})
	})
}
