package account

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/UruhaLushia/tomori/locales"
)

const (
	authority = "https://login.microsoftonline.com/consumers/oauth2/v2.0/"
	scope     = "XboxLive.signin offline_access"
)

type DeviceCode struct {
	Code            string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	ExpiresAt       time.Time
}

type oauthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (c *Client) Begin(ctx context.Context, clientID string) (DeviceCode, error) {
	var code DeviceCode
	if clientID == "" {
		return code, errors.New(locales.MicrosoftClientIdIsNotConfigured())
	}
	err := c.request(ctx, http.MethodPost, authority+"devicecode", "", url.Values{
		"client_id": {clientID}, "scope": {scope},
	}, &code)
	if err != nil {
		return code, err
	}
	u, err := url.Parse(code.VerificationURI)
	if code.Code == "" || code.UserCode == "" || code.ExpiresIn <= 0 || code.ExpiresIn > 3600 || err != nil || !validVerificationURI(u) {
		return DeviceCode{}, errors.New(locales.InvalidSignInCodeResponse())
	}
	code.Interval = max(5, min(code.Interval, 60))
	code.ExpiresAt = time.Now().Add(time.Duration(code.ExpiresIn) * time.Second)
	return code, nil
}

func validVerificationURI(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	switch u.Hostname() {
	case "microsoft.com", "www.microsoft.com", "login.microsoftonline.com":
		return true
	default:
		return false
	}
}

func (c *Client) Complete(ctx context.Context, clientID string, code DeviceCode) (Account, error) {
	ctx, cancel := context.WithDeadline(ctx, code.ExpiresAt)
	defer cancel()
	interval := time.Duration(max(5, code.Interval)) * time.Second
	for {
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if ctx.Err() == context.DeadlineExceeded {
				return Account{}, errors.New(locales.SignInCodeExpiredTryAgain())
			}
			return Account{}, ctx.Err()
		case <-timer.C:
		}
		var tokens oauthTokens
		err := c.request(ctx, http.MethodPost, authority+"token", "", url.Values{
			"client_id": {clientID}, "device_code": {code.Code},
			"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"},
		}, &tokens)
		switch {
		case err == nil:
			if tokens.RefreshToken == "" {
				return Account{}, errors.New(locales.NoRefreshTokenReturnedAuthorizeAgain())
			}
			return c.minecraft(ctx, clientID, tokens)
		case errorCode(err, "authorization_pending"):
			continue
		case errorCode(err, "slow_down"):
			interval += 5 * time.Second
		default:
			return Account{}, err
		}
	}
}

// Refresh returns rotated credentials even when a later Xbox/profile request
// fails. The caller must persist them before reporting that error.
func (c *Client) Refresh(ctx context.Context, a Account) (Account, error) {
	if err := a.Validate(); err != nil {
		return a, err
	}
	if a.Kind != Microsoft {
		return a, nil
	}
	var tokens oauthTokens
	if err := c.request(ctx, http.MethodPost, authority+"token", "", url.Values{
		"client_id": {a.ClientID}, "refresh_token": {a.RefreshToken},
		"grant_type": {"refresh_token"}, "scope": {scope},
	}, &tokens); err != nil {
		return a, err
	}
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = a.RefreshToken
	}
	a.RefreshToken = tokens.RefreshToken
	updated, err := c.minecraft(ctx, a.ClientID, tokens)
	if err != nil {
		return a, err
	}
	if updated.ID != a.ID {
		return a, errors.New(locales.ADifferentAccountWasReturnedSignInAgain())
	}
	return updated, nil
}

func (c *Client) Ensure(ctx context.Context, a Account) (Account, error) {
	if err := a.Validate(); err != nil {
		return a, err
	}
	if a.Kind == Yggdrasil {
		return c.EnsureYggdrasil(ctx, a)
	}
	if a.Kind == Microsoft && (a.AccessToken == "" || time.Until(a.ExpiresAt) < 2*time.Minute) {
		return c.Refresh(ctx, a)
	}
	return a, nil
}
