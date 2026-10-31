package account

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/locales"
)

type xboxToken struct {
	Token         string `json:"Token"`
	DisplayClaims struct {
		Users []struct {
			Hash string `json:"uhs"`
			XUID string `json:"xid"`
		} `json:"xui"`
	} `json:"DisplayClaims"`
}

func (c *Client) minecraft(ctx context.Context, clientID string, tokens oauthTokens) (Account, error) {
	if tokens.AccessToken == "" {
		return Account{}, errors.New(locales.MicrosoftSignInReturnedNoAccessToken())
	}
	var xbl xboxToken
	err := c.request(ctx, http.MethodPost, "https://user.auth.xboxlive.com/user/authenticate", "", map[string]any{
		"Properties":   map[string]string{"AuthMethod": "RPS", "SiteName": "user.auth.xboxlive.com", "RpsTicket": "d=" + tokens.AccessToken},
		"RelyingParty": "http://auth.xboxlive.com", "TokenType": "JWT",
	}, &xbl)
	if err != nil {
		return Account{}, fmt.Errorf(locales.XboxSignInFailed(), err)
	}
	if xbl.Token == "" {
		return Account{}, errors.New(locales.InvalidXboxSignInResponse())
	}
	var xsts xboxToken
	err = c.request(ctx, http.MethodPost, "https://xsts.auth.xboxlive.com/xsts/authorize", "", map[string]any{
		"Properties":   map[string]any{"SandboxId": "RETAIL", "UserTokens": []string{xbl.Token}},
		"RelyingParty": "rp://api.minecraftservices.com/", "TokenType": "JWT",
	}, &xsts)
	if err != nil {
		return Account{}, fmt.Errorf(locales.XboxAuthorizationFailed(), err)
	}
	if xsts.Token == "" || len(xsts.DisplayClaims.Users) == 0 || xsts.DisplayClaims.Users[0].Hash == "" {
		return Account{}, errors.New(locales.InvalidXboxAuthorizationResponse())
	}
	user := xsts.DisplayClaims.Users[0]
	var mc struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	err = c.request(ctx, http.MethodPost, "https://api.minecraftservices.com/authentication/login_with_xbox", "", map[string]string{
		"identityToken": "XBL3.0 x=" + user.Hash + ";" + xsts.Token,
	}, &mc)
	if err != nil {
		return Account{}, fmt.Errorf(locales.MinecraftSignInFailed(), err)
	}
	if mc.AccessToken == "" || mc.ExpiresIn <= 0 || mc.ExpiresIn > 86400*7 {
		return Account{}, errors.New(locales.InvalidMinecraftSignInResponse())
	}
	var entitlements struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := c.request(ctx, http.MethodGet, "https://api.minecraftservices.com/entitlements/mcstore", mc.AccessToken, nil, &entitlements); err != nil {
		return Account{}, fmt.Errorf(locales.GameOwnershipCheckFailed(), err)
	}
	if len(entitlements.Items) == 0 {
		return Account{}, errors.New(locales.AccountDoesNotOwnJavaEditionOrGamePassHasExpired())
	}
	var profile struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Skins []struct {
			State string `json:"state"`
			URL   string `json:"url"`
		} `json:"skins"`
	}
	if err := c.request(ctx, http.MethodGet, "https://api.minecraftservices.com/minecraft/profile", mc.AccessToken, nil, &profile); err != nil {
		var remote *apiError
		if errors.As(err, &remote) && remote.Status == http.StatusNotFound {
			return Account{}, errors.New(locales.NoJavaEditionProfileVerifyOwnershipAndCreateAPlayerProfile())
		}
		return Account{}, fmt.Errorf(locales.FailedToFetchGameProfile(), err)
	}
	id := strings.ToLower(strings.ReplaceAll(profile.ID, "-", ""))
	if !validUUID(id) || !validPlayerName(profile.Name) {
		return Account{}, errors.New(locales.InvalidGameProfile())
	}
	xuid := user.XUID
	if xuid == "" && len(xbl.DisplayClaims.Users) > 0 {
		xuid = xbl.DisplayClaims.Users[0].XUID
	}
	var skinURL string
	for _, skin := range profile.Skins {
		if skin.State == "ACTIVE" {
			skinURL = skin.URL
			break
		}
	}
	return Account{
		ID: "microsoft:" + id, Kind: Microsoft, Name: profile.Name, UUID: id, SkinURL: skinURL,
		ClientID: clientID, XUID: xuid, AccessToken: mc.AccessToken,
		RefreshToken: tokens.RefreshToken, ExpiresAt: time.Now().UTC().Add(time.Duration(mc.ExpiresIn) * time.Second),
	}, nil
}
