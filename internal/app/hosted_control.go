package app

import (
	"context"
	"errors"
	"net/http"

	"github.com/elythi0n/virta/internal/api"
	"github.com/elythi0n/virta/internal/hosted"
	"github.com/elythi0n/virta/internal/id"
)

// hostedAuthControl adapts the hosted.Manager to the api.HostedAuth interface, translating
// between the internal User type and the wire-friendly api.HostedUser type.
type hostedAuthControl struct {
	mgr *hosted.Manager
}

func newHostedAuthControl(store hosted.Store, gen id.Generator) *hostedAuthControl {
	return &hostedAuthControl{mgr: hosted.NewManager(store, gen)}
}

func (c *hostedAuthControl) Register(ctx context.Context, ip, email, displayName, password string) (api.HostedUser, string, error) {
	user, err := c.mgr.Register(ctx, email, displayName, password)
	if err != nil {
		return api.HostedUser{}, "", err
	}
	// Auto-login after registration.
	_, token, err := c.mgr.Login(ctx, ip, email, password)
	if err != nil {
		// Registration succeeded but auto-login failed — not critical, user can log in manually.
		return toAPIUser(user), "", nil
	}
	return toAPIUser(user), token, nil
}

func (c *hostedAuthControl) Login(ctx context.Context, ip, email, password string) (api.HostedUser, string, error) {
	user, token, err := c.mgr.Login(ctx, ip, email, password)
	if err != nil {
		return api.HostedUser{}, "", err
	}
	return toAPIUser(user), token, nil
}

func (c *hostedAuthControl) Logout(ctx context.Context, r *http.Request) error {
	return c.mgr.Logout(ctx, r)
}

func (c *hostedAuthControl) Resolve(ctx context.Context, r *http.Request) (api.HostedUser, error) {
	user, err := c.mgr.Resolve(ctx, r)
	if err != nil {
		if errors.Is(err, hosted.ErrUnauthorized) {
			return api.HostedUser{}, err
		}
		return api.HostedUser{}, err
	}
	return toAPIUser(user), nil
}

func toAPIUser(u hosted.User) api.HostedUser {
	return api.HostedUser{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName}
}
