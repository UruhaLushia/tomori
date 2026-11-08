package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/locales"
)

type YggdrasilServer struct{ URL, Name, Metadata string }

type YggdrasilProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type YggdrasilSession struct {
	Server      YggdrasilServer
	LoginName   string
	AccessToken string             `json:"accessToken"`
	ClientToken string             `json:"clientToken"`
	Profiles    []YggdrasilProfile `json:"availableProfiles"`
	Selected    *YggdrasilProfile  `json:"selectedProfile"`
	User        struct {
		ID         string                         `json:"id"`
		Properties []struct{ Name, Value string } `json:"properties"`
	} `json:"user"`
}

func validExternalName(value string, limit int) bool {
	return value != "" && len(value) <= limit && utf8.ValidString(value) && strings.TrimSpace(value) == value && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validCredential(value string) bool {
	return value != "" && len(value) <= 8192 && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func NormalizeYggdrasilURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 4096 || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || (u.Scheme != "https" && u.Scheme != "http") || strings.ContainsAny(u.Path, "\\\x00\n\r") {
		return "", errors.New(locales.InvalidAuthenticationServerAddress())
	}
	u.Host = strings.ToLower(u.Host)
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
		if u.RawPath != "" {
			u.RawPath += "/"
		}
	}
	return u.String(), nil
}

func (c *Client) YggdrasilServer(ctx context.Context, raw string, discover bool) (YggdrasilServer, error) {
	address, err := NormalizeYggdrasilURL(raw)
	if err != nil {
		return YggdrasilServer{}, err
	}
	for range 6 {
		response, release, err := c.publicAuthGet(ctx, address, discover)
		if err != nil {
			return YggdrasilServer{}, err
		}
		next := response.Header.Get("X-Authlib-Injector-API-Location")
		current := response.Request.URL
		if discover && next != "" {
			u, e := url.Parse(next)
			if e == nil {
				next, e = NormalizeYggdrasilURL(current.ResolveReference(u).String())
			}
			if e != nil || current.Scheme == "https" && strings.HasPrefix(next, "http:") {
				response.Body.Close()
				release()
				return YggdrasilServer{}, errors.New(locales.InvalidAuthenticationServerAddress())
			}
			if next != current.String() {
				response.Body.Close()
				release()
				address = next
				continue
			}
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
		response.Body.Close()
		release()
		if err != nil {
			if ctx.Err() != nil {
				return YggdrasilServer{}, ctx.Err()
			}
			return YggdrasilServer{}, errors.New(locales.FailedToReadSignInResponse())
		}
		var info struct {
			Meta      struct{ ServerName string } `json:"meta"`
			PublicKey string                      `json:"signaturePublickey"`
		}
		if len(raw) > 64<<10 || json.Unmarshal(raw, &info) != nil || info.PublicKey == "" || len(info.PublicKey) > 16<<10 {
			return YggdrasilServer{}, errors.New(locales.InvalidAuthenticationServerResponse())
		}
		name := info.Meta.ServerName
		if !validExternalName(name, 256) {
			name = current.Host
		}
		address, err = NormalizeYggdrasilURL(current.String())
		return YggdrasilServer{URL: address, Name: name, Metadata: string(raw)}, err
	}
	return YggdrasilServer{}, errors.New(locales.TooManyAuthenticationServerRedirects())
}

func (c *Client) publicAuthGet(ctx context.Context, address string, redirects bool) (*http.Response, func(), error) {
	release, err := c.network(ctx)
	if err != nil {
		return nil, nil, err
	}
	client := c.http.Clone()
	if redirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.User != nil || (req.URL.Scheme != "https" && req.URL.Scheme != "http") || via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return errors.New(locales.InvalidAuthenticationServerAddress())
			}
			return nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err == nil {
		req.Header.Set("Accept", "*/*")
		var response *http.Response
		response, err = client.Do(req)
		if err == nil {
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				return response, release, nil
			}
			response.Body.Close()
			release()
			return nil, nil, fmt.Errorf(locales.AuthenticationServerReturnedHttp(), response.StatusCode)
		}
	}
	release()
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	return nil, nil, errors.New(locales.CouldNotConnectToTheAuthenticationServer())
}

func (c *Client) LoginYggdrasil(ctx context.Context, server YggdrasilServer, login, password string) (YggdrasilSession, error) {
	login = strings.TrimSpace(login)
	if address, err := NormalizeYggdrasilURL(server.URL); err != nil || address != server.URL || !validExternalName(login, 512) || password == "" || len(password) > 4096 || strings.ContainsRune(password, 0) {
		return YggdrasilSession{}, errors.New(locales.EnterYourAccountAndPassword())
	}
	clientToken := rand.Text()
	var s YggdrasilSession
	err := c.request(ctx, http.MethodPost, server.URL+"authserver/authenticate", "", map[string]any{
		"agent": map[string]any{"name": "Minecraft", "version": 1}, "username": login, "password": password, "clientToken": clientToken, "requestUser": true,
	}, &s)
	if err != nil {
		return s, yggdrasilError(err, true)
	}
	s.Server, s.LoginName = server, login
	if err := s.validate(clientToken); err != nil {
		return YggdrasilSession{}, err
	}
	if s.Selected == nil && len(s.Profiles) == 0 {
		return YggdrasilSession{}, errors.New(locales.ThisAccountHasNoProfiles())
	}
	return s, nil
}

func (s YggdrasilSession) validate(clientToken string) error {
	if !validCredential(s.AccessToken) || s.ClientToken != clientToken || len(s.Profiles) > 128 || len(s.User.Properties) > 128 || len(s.User.ID) > 256 {
		return errors.New(locales.InvalidAuthenticationServerResponse())
	}
	validProfile := func(p YggdrasilProfile) bool { return validUUID(p.ID) && validExternalName(p.Name, 64) }
	if s.Selected != nil && !validProfile(*s.Selected) {
		return errors.New(locales.InvalidAuthenticationServerResponse())
	}
	seen := make(map[string]bool, len(s.Profiles))
	for _, p := range s.Profiles {
		if !validProfile(p) || seen[p.ID] {
			return errors.New(locales.InvalidAuthenticationServerResponse())
		}
		seen[p.ID] = true
	}
	return nil
}

func (c *Client) SelectYggdrasil(ctx context.Context, s YggdrasilSession, profileID string) (Account, error) {
	if s.Selected != nil {
		if profileID != "" && s.Selected.ID != profileID {
			return Account{}, errors.New(locales.TheOriginalProfileIsNoLongerAvailable())
		}
		return s.account()
	}
	for _, p := range s.Profiles {
		if p.ID != profileID {
			continue
		}
		var next YggdrasilSession
		err := c.request(ctx, http.MethodPost, s.Server.URL+"authserver/refresh", "", map[string]any{"accessToken": s.AccessToken, "clientToken": s.ClientToken, "selectedProfile": p, "requestUser": true}, &next)
		if err != nil {
			return Account{}, yggdrasilError(err, false)
		}
		next.Server, next.LoginName = s.Server, s.LoginName
		if err := next.validate(s.ClientToken); err != nil {
			return Account{}, err
		}
		if next.Selected == nil || next.Selected.ID != p.ID {
			return Account{}, errors.New(locales.TheOriginalProfileIsNoLongerAvailable())
		}
		return next.account()
	}
	return Account{}, errors.New(locales.ChooseAProfile())
}

func yggdrasilID(server, login, uuid string) string {
	sum := sha256.Sum256([]byte(server + "\x00" + login))
	return fmt.Sprintf("yggdrasil:%x:%s", sum, uuid)
}

func (s YggdrasilSession) account() (Account, error) {
	if s.Selected == nil {
		return Account{}, errors.New(locales.ChooseAProfile())
	}
	properties := make(map[string][]string)
	for _, p := range s.User.Properties {
		properties[p.Name] = append(properties[p.Name], p.Value)
	}
	raw, err := json.Marshal(properties)
	if err != nil || len(raw) > 32<<10 {
		return Account{}, errors.New(locales.InvalidAuthenticationServerResponse())
	}
	a := Account{Kind: Yggdrasil, Name: s.Selected.Name, UUID: s.Selected.ID, ServerURL: s.Server.URL, ServerName: s.Server.Name,
		LoginName: s.LoginName, AccessToken: s.AccessToken, ClientToken: s.ClientToken, UserID: s.User.ID, Properties: string(raw)}
	a.ID = yggdrasilID(a.ServerURL, a.LoginName, a.UUID)
	return a, a.Validate()
}

func (c *Client) EnsureYggdrasil(ctx context.Context, a Account) (Account, error) {
	err := c.request(ctx, http.MethodPost, a.ServerURL+"authserver/validate", "", map[string]string{"accessToken": a.AccessToken, "clientToken": a.ClientToken}, nil, http.StatusNoContent)
	if err == nil {
		return a, nil
	}
	var remote *apiError
	if !errors.As(err, &remote) || remote.Status != http.StatusForbidden && remote.Status != http.StatusUnauthorized {
		return a, err
	}
	var s YggdrasilSession
	err = c.request(ctx, http.MethodPost, a.ServerURL+"authserver/refresh", "", map[string]any{"accessToken": a.AccessToken, "clientToken": a.ClientToken, "requestUser": true}, &s)
	if err != nil {
		return a, yggdrasilError(err, false)
	}
	s.Server, s.LoginName = YggdrasilServer{URL: a.ServerURL, Name: a.ServerName}, a.LoginName
	if err := s.validate(a.ClientToken); err != nil {
		return a, err
	}
	if s.Selected == nil || s.Selected.ID != a.UUID {
		return a, errors.New(locales.TheOriginalProfileIsNoLongerAvailable())
	}
	updated, err := s.account()
	if err != nil {
		return a, err
	}
	return updated, nil
}

func yggdrasilError(err error, login bool) error {
	var remote *apiError
	if errors.As(err, &remote) && (remote.Status == http.StatusForbidden || remote.Status == http.StatusUnauthorized) {
		if login {
			return errors.New(locales.IncorrectAccountOrPassword())
		}
		return errors.New(locales.ExternalAccountExpiredSignInAgain())
	}
	return err
}
