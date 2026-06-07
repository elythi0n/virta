package app

import (
	"context"
	"fmt"

	"github.com/elythi0n/virta/internal/api"
	pluginhostpkg "github.com/elythi0n/virta/internal/pluginhost"
)

// pluginControl adapts the plugin Registry to the API's Plugins surface.
type pluginControl struct {
	reg       *pluginhostpkg.Registry
	installer *pluginhostpkg.Installer
}

func newPluginControl(reg *pluginhostpkg.Registry, installer *pluginhostpkg.Installer) *pluginControl {
	return &pluginControl{reg: reg, installer: installer}
}

func (c *pluginControl) List() []api.PluginInfo {
	entries := c.reg.List()
	out := make([]api.PluginInfo, 0, len(entries))
	for _, e := range entries {
		scopes := make([]string, 0, len(e.Manifest.Scopes))
		for _, s := range e.Manifest.Scopes {
			scopes = append(scopes, string(s))
		}
		out = append(out, api.PluginInfo{
			ID:          e.Manifest.ID,
			Name:        e.Manifest.Name,
			Version:     e.Manifest.Version,
			Publisher:   e.Manifest.Publisher,
			Description: e.Manifest.Description,
			Tags:        e.Manifest.Tags,
			State:       string(e.State),
			Error:       e.Error,
			BuiltIn:     e.Manifest.BuiltIn,
			Scopes:      scopes,
		})
	}
	return out
}

func (c *pluginControl) Enable(id string) error {
	return c.reg.Enable(context.Background(), id)
}

func (c *pluginControl) Disable(id string) error {
	return c.reg.Disable(context.Background(), id)
}

func (c *pluginControl) Install(url string) (api.PluginInfo, error) {
	if c.installer == nil {
		return api.PluginInfo{}, fmt.Errorf("installer not configured")
	}
	result, err := c.installer.Install(context.Background(), url)
	if err != nil {
		return api.PluginInfo{}, err
	}
	if regErr := c.reg.RegisterBuiltIn(result.Manifest); regErr != nil {
		// Already registered — still enable it.
		_ = regErr
	}
	if err := c.reg.Enable(context.Background(), result.Manifest.ID); err != nil {
		return api.PluginInfo{}, err
	}
	return api.PluginInfo{
		ID:          result.Manifest.ID,
		Name:        result.Manifest.Name,
		Version:     result.Manifest.Version,
		Publisher:   result.Manifest.Publisher,
		Description: result.Manifest.Description,
		Tags:        result.Manifest.Tags,
		State:       string(pluginhostpkg.StateEnabled),
		BuiltIn:     false,
	}, nil
}

func (c *pluginControl) Uninstall(id string) error {
	e, err := c.reg.Get(id)
	if err != nil {
		return err
	}
	if e.Manifest.BuiltIn {
		return fmt.Errorf("built-in plugins cannot be uninstalled")
	}
	if err := c.reg.Disable(context.Background(), id); err != nil {
		return err
	}
	if c.installer != nil && e.InstallDir != "" {
		return c.installer.Uninstall(e.InstallDir)
	}
	return nil
}
