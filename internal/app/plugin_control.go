package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/elythi0n/virta/internal/api"
	pluginhost "github.com/elythi0n/virta/internal/plugin/host"
)

// pluginControl adapts the plugin Registry to the API's Plugins surface.
type pluginControl struct {
	reg       *pluginhost.Registry
	installer *pluginhost.Installer
}

func newPluginControl(reg *pluginhost.Registry, installer *pluginhost.Installer) *pluginControl {
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
			HasConfig:   len(e.Manifest.Config) > 0,
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
	// Validate declared scopes are all known before accepting the plugin.
	if err := result.Manifest.Validate(); err != nil {
		return api.PluginInfo{}, fmt.Errorf("install: manifest validation failed: %w", err)
	}
	// Require explicit ScopeHTTP if the plugin contributes DataSources (they make network calls).
	if len(result.Manifest.Contributes.DataSources) > 0 && !result.Manifest.HasScope(pluginhost.ScopeHTTP) {
		return api.PluginInfo{}, fmt.Errorf(
			"install: plugin %q contributes DataSources but does not declare 'http' scope — installation rejected for safety",
			result.Manifest.ID,
		)
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
		State:       string(pluginhost.StateEnabled),
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

// GetDetail returns the full plugin manifest including config schema.
func (c *pluginControl) GetDetail(id string) (api.PluginDetail, error) {
	e, err := c.reg.Get(id)
	if err != nil {
		return api.PluginDetail{}, err
	}
	scopes := make([]string, 0, len(e.Manifest.Scopes))
	for _, s := range e.Manifest.Scopes {
		scopes = append(scopes, string(s))
	}
	info := api.PluginInfo{
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
	}
	var schema interface{}
	if len(e.Manifest.Config) > 0 {
		_ = json.Unmarshal(e.Manifest.Config, &schema) // ignore parse errors; nil schema is acceptable
	}
	return api.PluginDetail{PluginInfo: info, ConfigSchema: schema}, nil
}
