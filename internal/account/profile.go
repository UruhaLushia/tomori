package account

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/locales"
)

const profileURL = "https://api.minecraftservices.com/minecraft/profile"

type Skin struct {
	ID, State, URL, Variant string
}

type Cape struct {
	ID, State, URL, Alias string
}

type Profile struct {
	ID, Name string
	Skins    []Skin
	Capes    []Cape
}

func (p Profile) ActiveSkin() (Skin, bool) {
	for _, skin := range p.Skins {
		if skin.State == "ACTIVE" {
			return skin, true
		}
	}
	return Skin{}, false
}

func (p Profile) ActiveCape() string {
	for _, cape := range p.Capes {
		if cape.State == "ACTIVE" {
			return cape.ID
		}
	}
	return ""
}

func appearanceAccount(a Account) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if a.Kind != Microsoft {
		return errors.New(locales.AMicrosoftAccountIsRequired())
	}
	if a.AccessToken == "" || !a.ExpiresAt.After(time.Now()) {
		return errors.New(locales.SignInExpiredSignInAgain())
	}
	return nil
}

func (c *Client) Profile(ctx context.Context, a Account) (Profile, error) {
	var p Profile
	if err := appearanceAccount(a); err != nil {
		return p, err
	}
	if err := c.request(ctx, http.MethodGet, profileURL, a.AccessToken, nil, &p); err != nil {
		return p, err
	}
	p.ID = strings.ToLower(strings.ReplaceAll(p.ID, "-", ""))
	if p.ID != a.UUID || !validUUID(p.ID) || !validPlayerName(p.Name) || len(p.Skins) > 64 || len(p.Capes) > 128 {
		return Profile{}, errors.New(locales.InvalidGameProfile())
	}
	return p, nil
}

func (c *Client) UploadSkin(ctx context.Context, a Account, raw []byte, variant string) error {
	if err := appearanceAccount(a); err != nil {
		return err
	}
	if variant != "classic" && variant != "slim" {
		return errors.New(locales.InvalidSkinModel())
	}
	info, err := skinConfig(raw)
	if err != nil {
		return err
	}
	if info.Width != 64 {
		return errors.New(locales.UploadA6464Or6432PngSkin())
	}
	if _, err := decodeSkin(raw); err != nil {
		return err
	}
	var headers bytes.Buffer
	form := multipart.NewWriter(&headers)
	if err := form.WriteField("variant", variant); err != nil {
		return err
	}
	part := textproto.MIMEHeader{}
	part.Set("Content-Disposition", `form-data; name="file"; filename="skin.png"`)
	part.Set("Content-Type", "image/png")
	if _, err := form.CreatePart(part); err != nil {
		return err
	}
	prefix := headers.Len()
	if err := form.Close(); err != nil {
		return err
	}
	data := headers.Bytes()
	body := io.MultiReader(bytes.NewReader(data[:prefix]), bytes.NewReader(raw), bytes.NewReader(data[prefix:]))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, profileURL+"/skins", body)
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(data) + len(raw))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", form.FormDataContentType())
	return c.send(req, a.AccessToken, nil, 0)
}

func (c *Client) ResetSkin(ctx context.Context, a Account) error {
	if err := appearanceAccount(a); err != nil {
		return err
	}
	return c.request(ctx, http.MethodDelete, profileURL+"/skins/active", a.AccessToken, nil, nil)
}

func (c *Client) SetCape(ctx context.Context, a Account, id string) error {
	p, err := c.Profile(ctx, a)
	if err != nil {
		return err
	}
	if id == "" {
		return c.request(ctx, http.MethodDelete, profileURL+"/capes/active", a.AccessToken, nil, nil)
	}
	for _, cape := range p.Capes {
		if cape.ID == id {
			return c.request(ctx, http.MethodPut, profileURL+"/capes/active", a.AccessToken, map[string]string{"capeId": id}, nil)
		}
	}
	return errors.New(locales.ThisAccountDoesNotOwnThisCape())
}
