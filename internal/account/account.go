package account

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/locales"
)

type Kind string

const (
	Offline   Kind = "offline"
	Microsoft Kind = "microsoft"
	Yggdrasil Kind = "yggdrasil"
)

type Account struct {
	ID           string    `json:"id"`
	Kind         Kind      `json:"kind"`
	Name         string    `json:"name"`
	UUID         string    `json:"uuid"`
	SkinURL      string    `json:"skinUrl,omitempty"`
	ClientID     string    `json:"clientId,omitempty"`
	XUID         string    `json:"xuid,omitempty"`
	AccessToken  string    `json:"accessToken,omitempty"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`
	ServerURL    string    `json:"serverUrl,omitempty"`
	ServerName   string    `json:"serverName,omitempty"`
	LoginName    string    `json:"loginName,omitempty"`
	ClientToken  string    `json:"clientToken,omitempty"`
	UserID       string    `json:"userId,omitempty"`
	Properties   string    `json:"properties,omitempty"`
}

func validPlayerName(name string) bool {
	if len(name) < 1 || len(name) > 16 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
		default:
			return false
		}
	}
	return true
}

func NewOffline(name string) (Account, error) {
	name = strings.TrimSpace(name)
	if !validPlayerName(name) {
		return Account{}, errors.New(locales.PlayerNameMustContain116LettersDigitsOrUnderscores())
	}
	sum := md5.Sum([]byte("OfflinePlayer:" + name))
	sum[6] = sum[6]&0x0f | 0x30
	sum[8] = sum[8]&0x3f | 0x80
	id := hex.EncodeToString(sum[:])
	return Account{ID: "offline:" + id, Kind: Offline, Name: name, UUID: id}, nil
}

func (a Account) Validate() error {
	if a.ID == "" || !validUUID(a.UUID) || (a.Kind != Yggdrasil && !validPlayerName(a.Name)) {
		return errors.New(locales.AddAnAccountOrSignInAgain())
	}
	switch a.Kind {
	case Offline:
		expected, err := NewOffline(a.Name)
		if err != nil || a.UUID != expected.UUID || a.ID != expected.ID {
			return errors.New(locales.InvalidOfflineAccountAddItAgain())
		}
	case Microsoft:
		if a.ClientID == "" || a.RefreshToken == "" {
			return errors.New(locales.AccountIsNoLongerValidSignInAgain())
		}
	case Yggdrasil:
		server, err := NormalizeYggdrasilURL(a.ServerURL)
		if err != nil || server != a.ServerURL || !validExternalName(a.Name, 64) || !validExternalName(a.LoginName, 512) || a.ID != yggdrasilID(server, a.LoginName, a.UUID) || !validCredential(a.AccessToken) || !validCredential(a.ClientToken) {
			return errors.New(locales.ExternalAccountExpiredSignInAgain())
		}
		if a.Properties != "" && (len(a.Properties) > 32<<10 || !strings.HasPrefix(strings.TrimSpace(a.Properties), "{") || !json.Valid([]byte(a.Properties))) {
			return errors.New(locales.InvalidAuthenticationServerResponse())
		}
	default:
		return errors.New(locales.UnsupportedAccountType())
	}
	return nil
}

func validUUID(s string) bool {
	if len(s) != 32 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func (a Account) Initial() string {
	if a.Name == "" {
		return "?"
	}
	return strings.ToUpper(string([]rune(a.Name)[0]))
}

func (a Account) Label() string {
	if a.ID == "" {
		return locales.NoAccountSelected()
	}
	if a.Kind == Microsoft {
		return locales.MicrosoftAccount()
	}
	if a.Kind == Yggdrasil {
		if a.ServerName != "" {
			return locales.ExternalLogin() + " · " + a.ServerName
		}
		return a.ServerURL
	}
	return locales.OfflineAccount()
}

func (a Account) Redact(text string) string {
	for _, secret := range []string{a.AccessToken, a.RefreshToken, a.ClientToken} {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	return text
}

func (a Account) SessionToken() string {
	if a.Kind == Yggdrasil {
		return a.AccessToken
	}
	return a.RefreshToken
}
