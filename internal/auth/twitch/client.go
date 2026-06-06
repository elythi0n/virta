// Package twitch implements Twitch's OAuth Device Code Grant (DCF) — the flow for public
// native clients with no client secret (ADR-008) — plus token refresh with single-use
// rotation and token validation. The HTTP client and endpoint URLs are injectable so the whole
// flow is tested offline; the live endpoints are exercised manually (tracked in live-debt).
package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/clock"
)

// Default Twitch OAuth endpoints.
const (
	defaultDeviceURL   = "https://id.twitch.tv/oauth2/device"
	defaultTokenURL    = "https://id.twitch.tv/oauth2/token"
	defaultValidateURL = "https://id.twitch.tv/oauth2/validate"

	deviceGrant = "urn:ietf:params:oauth:grant-type:device_code"
)

// Poll outcomes the caller must handle distinctly from success.
var (
	// ErrAuthorizationPending: the user hasn't authorized yet — keep polling.
	ErrAuthorizationPending = errors.New("twitch: authorization pending")
	// ErrSlowDown: poll less often (Twitch is rate-limiting the poll).
	ErrSlowDown = errors.New("twitch: slow down")
	// ErrExpired: the device code expired before authorization.
	ErrExpired = errors.New("twitch: device code expired")
	// ErrAccessDenied: the user declined authorization.
	ErrAccessDenied = errors.New("twitch: access denied")
)

// DeviceAuth is the device-flow start response shown to the user.
type DeviceAuth struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	Interval        int // seconds between polls
	ExpiresIn       int // seconds until the device code expires
}

// Token is a stored credential set. RefreshToken is single-use (Twitch rotates it on refresh).
type Token struct {
	Access    string    `json:"access"`
	Refresh   string    `json:"refresh"`
	ExpiresAt time.Time `json:"expires_at"`
	Scopes    []string  `json:"scopes"`
}

// Identity is who a token belongs to (from /oauth2/validate).
type Identity struct {
	UserID string
	Login  string
	Scopes []string
}

// Client talks to Twitch's OAuth endpoints with a public client id (no secret). The client id is
// read through a provider on each call, so it can be configured at runtime (settings UI) without
// rebuilding the client.
type Client struct {
	clientID    func() string
	http        *http.Client
	clk         clock.Clock
	deviceURL   string
	tokenURL    string
	validateURL string
}

// NewClient builds a client reading its public client id from clientID on each call. A nil http
// client uses a sane default with a timeout.
func NewClient(clientID func() string, hc *http.Client, clk clock.Clock) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		clientID:    clientID,
		http:        hc,
		clk:         clk,
		deviceURL:   defaultDeviceURL,
		tokenURL:    defaultTokenURL,
		validateURL: defaultValidateURL,
	}
}

// SetEndpoints overrides the endpoint URLs (tests point these at a local server).
func (c *Client) SetEndpoints(device, token, validate string) {
	c.deviceURL, c.tokenURL, c.validateURL = device, token, validate
}

// StartDevice begins the device flow, returning the code to show the user.
func (c *Client) StartDevice(ctx context.Context, scopes []string) (DeviceAuth, error) {
	form := url.Values{"client_id": {c.clientID()}, "scopes": {strings.Join(scopes, " ")}}
	var body struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := c.postForm(ctx, c.deviceURL, form, &body); err != nil {
		return DeviceAuth{}, err
	}
	if body.Interval <= 0 {
		body.Interval = 5
	}
	return DeviceAuth{
		DeviceCode:      body.DeviceCode,
		UserCode:        body.UserCode,
		VerificationURI: body.VerificationURI,
		Interval:        body.Interval,
		ExpiresIn:       body.ExpiresIn,
	}, nil
}

// PollToken polls once for the device authorization. It returns a Token on success, or one of
// the sentinel errors (pending/slow-down/expired/denied) the poller distinguishes.
func (c *Client) PollToken(ctx context.Context, deviceCode string) (Token, error) {
	form := url.Values{
		"client_id":   {c.clientID()},
		"device_code": {deviceCode},
		"grant_type":  {deviceGrant},
	}
	return c.tokenRequest(ctx, form)
}

// Refresh exchanges a refresh token for a fresh token set (Twitch rotates the refresh token).
func (c *Client) Refresh(ctx context.Context, refreshToken string) (Token, error) {
	form := url.Values{
		"client_id":     {c.clientID()},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	return c.tokenRequest(ctx, form)
}

// tokenRequest performs a token-endpoint POST and maps the response to a Token or a sentinel.
func (c *Client) tokenRequest(ctx context.Context, form url.Values) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		switch {
		case strings.Contains(e.Message, "authorization_pending"):
			return Token{}, ErrAuthorizationPending
		case strings.Contains(e.Message, "slow_down"):
			return Token{}, ErrSlowDown
		case strings.Contains(e.Message, "expired"):
			return Token{}, ErrExpired
		case strings.Contains(e.Message, "denied"):
			return Token{}, ErrAccessDenied
		default:
			return Token{}, fmt.Errorf("twitch: token request: status %d: %s", resp.StatusCode, e.Message)
		}
	}

	var ok struct {
		Access    string   `json:"access_token"`
		Refresh   string   `json:"refresh_token"`
		ExpiresIn int      `json:"expires_in"`
		Scope     []string `json:"scope"`
	}
	if err := json.Unmarshal(raw, &ok); err != nil {
		return Token{}, fmt.Errorf("twitch: decode token: %w", err)
	}
	return Token{
		Access:    ok.Access,
		Refresh:   ok.Refresh,
		ExpiresAt: c.clk.Now().Add(time.Duration(ok.ExpiresIn) * time.Second),
		Scopes:    ok.Scope,
	}, nil
}

// Validate resolves the account a token belongs to (and confirms it's live).
func (c *Client) Validate(ctx context.Context, accessToken string) (Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.validateURL, nil)
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Authorization", "OAuth "+accessToken) // validate uses the OAuth scheme
	resp, err := c.http.Do(req)
	if err != nil {
		return Identity{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("twitch: validate: status %d", resp.StatusCode)
	}
	var body struct {
		UserID string   `json:"user_id"`
		Login  string   `json:"login"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Identity{}, fmt.Errorf("twitch: decode validate: %w", err)
	}
	return Identity{UserID: body.UserID, Login: body.Login, Scopes: body.Scopes}, nil
}

func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("twitch: %s: status %d", endpoint, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}
