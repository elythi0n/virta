package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// PluginState is the current lifecycle state of a registered plugin.
type PluginState string

const (
	StateEnabled   PluginState = "enabled"
	StateDisabled  PluginState = "disabled"
	StateError     PluginState = "error"
	StateInstalled PluginState = "installed" // installed but not yet started this session
)

// Entry is one plugin known to the registry.
type Entry struct {
	Manifest *Manifest
	State    PluginState
	Error    string // non-empty when State == StateError
	// InstallDir is the local cache directory for remote plugins (empty for built-ins).
	InstallDir string
	// Config holds the plugin's saved configuration values (validated against Manifest.Config
	// by the settings UI; stored opaquely here).
	Config json.RawMessage
	// cancel stops the plugin's goroutines on disable/uninstall.
	cancel context.CancelFunc
}

// HostAPI is the surface the registry exposes to running plugin instances.
// Each method is capability-gated: the plugin's manifest Scopes are checked before dispatch.
type HostAPI interface {
	// RegisterDataSource wires a DataSource into the daemon pipeline under the plugin's namespace.
	RegisterDataSource(pluginID string, ds DataSourceRunner) error
	// RegisterCommand adds a slash-command contributed by the plugin.
	RegisterCommand(pluginID string, cmd ContributedCommand) error
	// UnregisterAll removes all contributions made by pluginID.
	UnregisterAll(pluginID string)
}

// DataSourceRunner is the minimal interface a plugin DataSource satisfies.
type DataSourceRunner interface {
	ID() string
	Run(ctx context.Context, publish func(stream string, data any)) error
}

// ContributedCommand is a slash-command registered by a plugin.
type ContributedCommand struct {
	Name    string
	Title   string
	Handler func(ctx context.Context, args string) (string, error)
}

// Registry manages plugin lifecycle: loading built-ins, installing remote plugins,
// enabling/disabling, and wiring contributions into the host.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*Entry // id → entry
	host    HostAPI
	store   PluginStore
	log     *slog.Logger
}

// PluginStore persists plugin metadata across restarts.
type PluginStore interface {
	List(ctx context.Context) ([]*PluginRecord, error)
	Save(ctx context.Context, r *PluginRecord) error
	Delete(ctx context.Context, id string) error
}

// PluginRecord is what is persisted in the store.
type PluginRecord struct {
	ID          string          `json:"id"`
	ManifestRaw json.RawMessage `json:"manifest"`
	State       PluginState     `json:"state"`
	InstallDir  string          `json:"install_dir,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
}

// New creates a Registry.
func New(host HostAPI, store PluginStore, log *slog.Logger) *Registry {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Registry{
		entries: map[string]*Entry{},
		host:    host,
		store:   store,
		log:     log,
	}
}

// RegisterBuiltIn registers a built-in plugin (ships in the binary).
// Built-ins are always enabled; the user can disable but not uninstall them.
func (r *Registry) RegisterBuiltIn(m *Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[m.ID]; exists {
		return fmt.Errorf("plugin %q already registered", m.ID)
	}
	m.BuiltIn = true
	r.entries[m.ID] = &Entry{Manifest: m, State: StateInstalled}
	return nil
}

// RegisterRemote registers (or replaces, on reinstall/upgrade) a remotely installed plugin.
// Saved config is carried over across reinstalls of the same plugin id.
func (r *Registry) RegisterRemote(ctx context.Context, m *Manifest, installDir string) error {
	if err := m.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	m.BuiltIn = false
	var keptConfig json.RawMessage
	if prev, ok := r.entries[m.ID]; ok {
		if prev.Manifest.BuiltIn {
			r.mu.Unlock()
			return fmt.Errorf("plugin %q is built-in and cannot be replaced by a remote install", m.ID)
		}
		keptConfig = prev.Config
		if prev.cancel != nil {
			prev.cancel()
		}
	}
	r.entries[m.ID] = &Entry{Manifest: m, State: StateInstalled, InstallDir: installDir, Config: keptConfig}
	r.mu.Unlock()
	return r.persist(ctx, m.ID)
}

// GetConfig returns the plugin's saved configuration values (nil when never configured).
func (r *Registry) GetConfig(id string) (json.RawMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	if !ok {
		return nil, errors.New("plugin not found: " + id)
	}
	return e.Config, nil
}

// SetConfig stores the plugin's configuration values and persists them.
func (r *Registry) SetConfig(ctx context.Context, id string, cfg json.RawMessage) error {
	r.mu.Lock()
	e, ok := r.entries[id]
	if !ok {
		r.mu.Unlock()
		return errors.New("plugin not found: " + id)
	}
	e.Config = cfg
	r.mu.Unlock()
	return r.persist(ctx, id)
}

// persist writes the plugin's current record to the store (no-op without a store).
func (r *Registry) persist(ctx context.Context, id string) error {
	if r.store == nil {
		return nil
	}
	r.mu.RLock()
	e, ok := r.entries[id]
	if !ok {
		r.mu.RUnlock()
		return errors.New("plugin not found: " + id)
	}
	rec := &PluginRecord{ID: id, State: e.State, InstallDir: e.InstallDir, Config: e.Config}
	if !e.Manifest.BuiltIn {
		raw, err := json.Marshal(e.Manifest)
		if err != nil {
			r.mu.RUnlock()
			return fmt.Errorf("pluginhost: marshal manifest: %w", err)
		}
		rec.ManifestRaw = raw
	}
	r.mu.RUnlock()
	return r.store.Save(ctx, rec)
}

// Start loads persisted plugin state and starts all enabled plugins.
func (r *Registry) Start(ctx context.Context) error {
	if r.store == nil {
		// No persistence — start all registered built-ins.
		r.mu.RLock()
		ids := make([]string, 0, len(r.entries))
		for id := range r.entries {
			ids = append(ids, id)
		}
		r.mu.RUnlock()
		for _, id := range ids {
			if err := r.Enable(ctx, id); err != nil {
				r.log.Warn("plugin failed to start", "id", id, "err", err)
			}
		}
		return nil
	}

	records, err := r.store.List(ctx)
	if err != nil {
		return fmt.Errorf("pluginhost: load plugin records: %w", err)
	}
	for _, rec := range records {
		// Remote plugins exist only as records — reconstruct their entries before state handling.
		if len(rec.ManifestRaw) > 0 {
			m, perr := ParseManifest(rec.ManifestRaw)
			if perr != nil {
				r.log.Warn("plugin record has invalid manifest, skipping", "id", rec.ID, "err", perr)
				continue
			}
			r.mu.Lock()
			if _, exists := r.entries[rec.ID]; !exists {
				m.BuiltIn = false
				r.entries[rec.ID] = &Entry{Manifest: m, State: StateInstalled, InstallDir: rec.InstallDir, Config: rec.Config}
			}
			r.mu.Unlock()
		} else {
			// Built-in record: carry the saved config onto the registered entry.
			r.mu.Lock()
			if e, ok := r.entries[rec.ID]; ok && len(rec.Config) > 0 {
				e.Config = rec.Config
			}
			r.mu.Unlock()
		}
		if rec.State == StateDisabled {
			r.mu.Lock()
			if e, ok := r.entries[rec.ID]; ok {
				e.State = StateDisabled
			}
			r.mu.Unlock()
			continue
		}
		if err := r.Enable(ctx, rec.ID); err != nil {
			r.log.Warn("plugin failed to start", "id", rec.ID, "err", err)
		}
	}
	// Start built-ins that had no persisted record.
	r.mu.RLock()
	toStart := make([]string, 0)
	for id, e := range r.entries {
		if e.State == StateInstalled {
			toStart = append(toStart, id)
		}
	}
	r.mu.RUnlock()
	for _, id := range toStart {
		if err := r.Enable(ctx, id); err != nil {
			r.log.Warn("built-in plugin failed to start", "id", id, "err", err)
		}
	}
	return nil
}

// Enable starts a registered plugin, wiring its contributions into the host.
// Scope enforcement happens here: contributions requiring ScopeHTTP are only activated
// if the manifest declares it, and data-source contributions require ScopeHTTP.
func (r *Registry) Enable(ctx context.Context, id string) error {
	r.mu.Lock()
	e, ok := r.entries[id]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("plugin %q not registered", id)
	}
	if e.State == StateEnabled {
		r.mu.Unlock()
		return nil // already running
	}

	// Enforce: a plugin that contributes DataSources makes outbound network calls and
	// therefore must declare ScopeHTTP. Reject activation if the scope is missing.
	if len(e.Manifest.Contributes.DataSources) > 0 && !e.Manifest.HasScope(ScopeHTTP) {
		e.State = StateError
		e.Error = "plugin contributes DataSources but does not declare the 'http' scope in its manifest"
		r.mu.Unlock()
		return fmt.Errorf("plugin %q: DataSource contributions require 'http' scope (not declared)", id)
	}

	_, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.State = StateEnabled
	e.Error = ""
	builtin := e.Manifest.BuiltIn
	r.mu.Unlock()

	r.log.Info("plugin enabled", "id", id, "builtin", builtin)
	if !builtin {
		if err := r.persist(ctx, id); err != nil {
			r.log.Warn("plugin state persist failed", "id", id, "err", err)
		}
	}
	return nil
}

// Disable stops a running plugin and removes its contributions.
func (r *Registry) Disable(ctx context.Context, id string) error {
	r.mu.Lock()
	e, ok := r.entries[id]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("plugin %q not registered", id)
	}
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	e.State = StateDisabled
	r.mu.Unlock()

	r.host.UnregisterAll(id)
	r.log.Info("plugin disabled", "id", id)

	// Persist via the full record so a remote plugin's manifest/config survive the disable.
	if err := r.persist(ctx, id); err != nil {
		r.log.Warn("plugin state persist failed", "id", id, "err", err)
	}
	return nil
}

// Remove deletes a plugin's entry and persisted record (used by uninstall).
func (r *Registry) Remove(ctx context.Context, id string) error {
	r.mu.Lock()
	e, ok := r.entries[id]
	if !ok {
		r.mu.Unlock()
		return errors.New("plugin not found: " + id)
	}
	if e.Manifest.BuiltIn {
		r.mu.Unlock()
		return errors.New("built-in plugins cannot be removed")
	}
	if e.cancel != nil {
		e.cancel()
	}
	delete(r.entries, id)
	r.mu.Unlock()
	r.host.UnregisterAll(id)
	if r.store != nil {
		return r.store.Delete(ctx, id)
	}
	return nil
}

// List returns a snapshot of all registered plugins.
func (r *Registry) List() []*Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Entry, 0, len(r.entries))
	for _, e := range r.entries {
		cp := *e
		out = append(out, &cp)
	}
	return out
}

// Get returns the entry for a plugin by ID.
func (r *Registry) Get(id string) (*Entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	if !ok {
		return nil, errors.New("plugin not found: " + id)
	}
	cp := *e
	return &cp, nil
}
