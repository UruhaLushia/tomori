package account

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/network/download"
	httpclient "github.com/UruhaLushia/tomori/internal/network/http"
	"github.com/UruhaLushia/tomori/internal/network/proxy"
	"github.com/UruhaLushia/tomori/locales"
)

// DefaultClientID is Tomori's approved public client ID. It can be overridden
// with -ldflags -X when building a private distribution.
var DefaultClientID = "78432422-b913-4a45-ba48-3f6699f1e6f3"

type Client struct {
	http   *httpclient.Client
	budget *download.Budget
}

func NewClient(proxy proxy.Proxy, budgets ...*download.Budget) *Client {
	c := &Client{http: httpclient.New(proxy, 30*time.Second)}
	// Authentication credentials must never follow a server redirect.
	c.http.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if len(budgets) > 0 {
		c.budget = budgets[0]
	}
	return c
}

func (c *Client) Close() { c.http.Close() }

type apiError struct {
	Status     int     `json:"-"`
	Endpoint   string  `json:"-"`
	InvalidApp bool    `json:"-"`
	Code       string  `json:"error"`
	ErrorCodes []int64 `json:"error_codes"`
	XErr       int64   `json:"XErr"`
}

func (e *apiError) Error() string {
	if e.InvalidApp {
		return locales.ApplicationHasNotBeenApprovedForMinecraftApiAccess()
	}
	switch e.XErr {
	case 2148916227:
		return locales.XboxAccountIsBanned()
	case 2148916229, 2148916237:
		return locales.SignInBlockedByXboxParentalControls()
	case 2148916233:
		return locales.CreateAnXboxProfileAtXboxComFirst()
	case 2148916234:
		return locales.AcceptTheTermsOfServiceAtXboxComFirst()
	case 2148916235:
		return locales.XboxLiveIsUnavailableInThisRegion()
	case 2148916236, 2148916238, 2148916239:
		return locales.ChildAccountMustJoinAnXboxFamilyAndReceivePermission()
	}
	switch e.Code {
	case "invalid_grant", "interaction_required":
		return locales.SignInExpiredSignInAgain()
	case "invalid_client", "unauthorized_client":
		return locales.InvalidClientIdOrPublicClientSignInIsDisabled()
	case "authorization_declined", "access_denied":
		return locales.SignInDenied()
	case "expired_token":
		return locales.SignInCodeExpiredTryAgain()
	}
	if e.Status == http.StatusUnauthorized {
		return locales.SignInExpiredSignInAgain()
	}
	if e.Status == http.StatusForbidden {
		return locales.ThisAccountCannotPerformThisAction()
	}
	// Do not return remote error descriptions: they may contain credentials.
	return fmt.Sprintf(locales.SignInServiceReturnedHttp(), e.Status)
}

// Diagnostic includes only protocol codes and the endpoint, never response
// descriptions, login codes or credentials.
func Diagnostic(err error) string {
	var remote *apiError
	if !errors.As(err, &remote) {
		return err.Error()
	}
	var info strings.Builder
	fmt.Fprintf(&info, "%s [endpoint=%s HTTP=%d", err, remote.Endpoint, remote.Status)
	switch remote.Code {
	case "invalid_grant", "interaction_required", "invalid_client", "unauthorized_client",
		"authorization_declined", "access_denied", "expired_token", "invalid_scope",
		"invalid_request", "authorization_pending", "slow_down", "bad_verification_code",
		"invalid_resource", "temporarily_unavailable", "server_error":
		fmt.Fprintf(&info, " code=%s", remote.Code)
	}
	for _, code := range remote.ErrorCodes[:min(5, len(remote.ErrorCodes))] {
		fmt.Fprintf(&info, " AADSTS%d", code)
	}
	if remote.XErr != 0 {
		fmt.Fprintf(&info, " XErr=%d", remote.XErr)
	}
	if remote.InvalidApp {
		info.WriteString(" reason=InvalidAppRegistration")
	}
	return info.String() + "]"
}

func (c *Client) request(ctx context.Context, method, endpoint, bearer string, body any, result any, status ...int) error {
	var data []byte
	contentType := "application/json"
	var err error
	if form, ok := body.(url.Values); ok {
		data, contentType = []byte(form.Encode()), "application/x-www-form-urlencoded"
	} else if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	expected := 0
	if len(status) > 0 {
		expected = status[0]
	}
	return c.send(req, bearer, result, expected)
}

func (c *Client) network(ctx context.Context) (func(), error) {
	if c.budget == nil {
		return func() {}, ctx.Err()
	}
	return c.budget.NetworkPermit(ctx)
}

func (c *Client) send(req *http.Request, bearer string, result any, expected int) error {
	ctx := req.Context()
	release, err := c.network(ctx)
	if err != nil {
		return err
	}
	defer release()
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New(locales.CannotConnectToSignInServiceCheckYourNetwork())
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New(locales.FailedToReadSignInResponse())
	}
	if len(raw) > 1<<20 {
		return errors.New(locales.SignInResponseTooLarge())
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || expected != 0 && resp.StatusCode != expected {
		e := &apiError{Status: resp.StatusCode, Endpoint: req.URL.Host + req.URL.Path}
		_ = json.Unmarshal(raw, e)
		if req.URL.Host == "api.minecraftservices.com" {
			var detail struct {
				Message string `json:"errorMessage"`
			}
			if json.Unmarshal(raw, &detail) == nil {
				e.InvalidApp = strings.Contains(strings.ToLower(detail.Message), "invalid app registration")
			}
		}
		return e
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return errors.New(locales.InvalidSignInResponseFormat())
	}
	return nil
}

func errorCode(err error, code string) bool {
	var remote *apiError
	return errors.As(err, &remote) && remote.Code == code
}
