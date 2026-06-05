// Package kick implements Kick's OAuth 2.1 authorization-code flow with PKCE (ADR-007/004).
// Unlike Twitch's device flow, Kick uses a browser redirect to a loopback URL; this package
// builds the authorize URL, exchanges the code (with the PKCE verifier), refreshes tokens, and
// resolves the account identity. The HTTP client and endpoints are injectable so the flow is
// tested offline; live Kick endpoints (and the client-secret-on-PKCE question) are tracked in
// live-debt.
package kick

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/clock"
)

// Default Kick OAuth + API endpoints.
const (
	defaultAuthURL  = "https://id.kick.com/oauth/authorize"
	defaultTokenURL = "https://id.kick.com/oauth/token"
	defaultUsersURL = "https://api.kick.com/public/v1/users"
)

// DefaultScopes are the v1 chat scopes.
var DefaultScopes = []string{"user:read", "chat:write", "events:subscribe"}

// Token is a stored credential set.
type Token struct {
	Access    string    `json:"access"`
	Refresh   string    `json:"refresh"`
	ExpiresAt time.Time `json:"expires_at"`
	Scopes    []string  `json:"scopes"`
}

// Identity is who a token belongs to.
type Identity struct {
	UserID string
	Login  string
}

// Client talks to Kick's OAuth endpoints.
type Client struct {
	clientID     string
	clientSecret string // ⚠ Kick may require this even with PKCE; empty = pure PKCE (docs 04)
	http         *http.Client
	clk          clock.Clock
	authURL      string
	tokenURL     string
	usersURL     string
}

// NewClient builds a Kick OAuth client. clientSecret may be empty (pure PKCE).
func NewClient(clientID, clientSecret string, hc *http.Client, clk clock.Clock) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		http:         hc,
		clk:          clk,
		authURL:      defaultAuthURL,
		tokenURL:     defaultTokenURL,
		usersURL:     defaultUsersURL,
	}
}

// SetEndpoints overrides the endpoint URLs (tests point these at a local server).
func (c *Client) SetEndpoints(auth, token, users string) {
	c.authURL, c.tokenURL, c.usersURL = auth, token, users
}

// AuthorizeURL builds the URL the user opens to authorize, embedding the PKCE challenge and
// state and the loopback redirect.
func (c *Client) AuthorizeURL(redirectURI string, scopes []string, challenge, state string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {c.clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {strings.Join(scopes, " ")},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	return c.authURL + "?" + q.Encode()
}

// Exchange swaps an authorization code (with its PKCE verifier) for tokens.
func (c *Client) Exchange(ctx context.Context, code, verifier, redirectURI string) (Token, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {c.clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	c.maybeSecret(form)
	return c.tokenRequest(ctx, form)
}

// Refresh exchanges a refresh token for a fresh token set.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (Token, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {c.clientID},
		"refresh_token": {refreshToken},
	}
	c.maybeSecret(form)
	return c.tokenRequest(ctx, form)
}

func (c *Client) maybeSecret(form url.Values) {
	if c.clientSecret != "" {
		form.Set("client_secret", c.clientSecret)
	}
}

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
		return Token{}, fmt.Errorf("kick: token request: status %d: %s", resp.StatusCode, string(raw))
	}
	var ok struct {
		Access    string `json:"access_token"`
		Refresh   string `json:"refresh_token"`
		ExpiresIn int    `json:"expires_in"`
		Scope     string `json:"scope"`
	}
	if err := json.Unmarshal(raw, &ok); err != nil {
		return Token{}, fmt.Errorf("kick: decode token: %w", err)
	}
	return Token{
		Access:    ok.Access,
		Refresh:   ok.Refresh,
		ExpiresAt: c.clk.Now().Add(time.Duration(ok.ExpiresIn) * time.Second),
		Scopes:    strings.Fields(ok.Scope),
	}, nil
}

// Identity resolves the account a token belongs to via the public users endpoint. The exact
// response shape is unverified (docs 04 ⚠); decoding is tolerant.
func (c *Client) Identity(ctx context.Context, accessToken string) (Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.usersURL, nil)
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return Identity{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("kick: identity: status %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			UserID json.Number `json:"user_id"`
			Name   string      `json:"name"`
			Slug   string      `json:"slug"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Identity{}, fmt.Errorf("kick: decode identity: %w", err)
	}
	if len(body.Data) == 0 {
		return Identity{}, fmt.Errorf("kick: identity: empty response")
	}
	login := body.Data[0].Slug
	if login == "" {
		login = body.Data[0].Name
	}
	return Identity{UserID: body.Data[0].UserID.String(), Login: login}, nil
}

// pkceChallenge derives the S256 code challenge from a verifier.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
