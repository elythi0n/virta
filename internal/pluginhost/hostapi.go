package pluginhost

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// defaultHostAPI implements HostAPI, wiring plugin contributions into the daemon.
// It is the bridge between the plugin registry and the daemon's command dispatch, panel registry,
// and DataSource runner — exposed to plugins only within their declared scopes.
type defaultHostAPI struct {
	mu       sync.RWMutex
	// dataSources holds running DataSource goroutines by pluginID.
	dataSources map[string]context.CancelFunc
	// commands holds contributed slash-commands by "pluginID/commandName".
	commands map[string]ContributedCommand
	// dsRunner is called to wire a DataSource into the pipeline.
	dsRunner func(ctx context.Context, ds DataSourceRunner) error
	// commandRegistrar is called to add a slash-command to the dispatch table.
	commandRegistrar func(pluginID, name, title string, fn func(ctx context.Context, args string) (string, error))
}

// NewHostAPI builds a HostAPI backed by the provided wiring functions.
// dsRunner is called when a plugin registers a DataSource.
// commandRegistrar is called when a plugin registers a slash-command.
func NewHostAPI(
	dsRunner func(ctx context.Context, ds DataSourceRunner) error,
	commandRegistrar func(pluginID, name, title string, fn func(ctx context.Context, args string) (string, error)),
) HostAPI {
	return &defaultHostAPI{
		dataSources:      map[string]context.CancelFunc{},
		commands:         map[string]ContributedCommand{},
		dsRunner:         dsRunner,
		commandRegistrar: commandRegistrar,
	}
}

func (h *defaultHostAPI) RegisterDataSource(pluginID string, ds DataSourceRunner) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.dsRunner == nil {
		return fmt.Errorf("hostapi: no DataSource runner configured")
	}
	key := pluginID + "/" + ds.ID()
	// Cancel any previously running instance under the same key before starting a new one.
	if old, exists := h.dataSources[key]; exists {
		old()
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.dataSources[key] = cancel
	go func() {
		if err := h.dsRunner(ctx, ds); err != nil && ctx.Err() == nil {
			// DataSource errored without being cancelled — the caller will see this if
			// it monitors the pipeline's log. We cannot panic: the host must survive a
			// misbehaving plugin DataSource.
		}
	}()
	return nil
}

func (h *defaultHostAPI) RegisterCommand(pluginID string, cmd ContributedCommand) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := pluginID + "/" + cmd.Name
	h.commands[key] = cmd
	if h.commandRegistrar != nil {
		h.commandRegistrar(pluginID, cmd.Name, cmd.Title, cmd.Handler)
	}
	return nil
}

func (h *defaultHostAPI) UnregisterAll(pluginID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	prefix := pluginID + "/"
	// Cancel and remove all DataSources owned by this plugin.
	for key, cancel := range h.dataSources {
		if strings.HasPrefix(key, prefix) {
			cancel()
			delete(h.dataSources, key)
		}
	}
	// Remove all commands contributed by this plugin.
	for key := range h.commands {
		if strings.HasPrefix(key, prefix) {
			delete(h.commands, key)
		}
	}
}

// DispatchCommand routes a slash-command call to the owning plugin, if one is registered.
// Returns (response, true) on match, ("", false) when not a plugin command.
func (h *defaultHostAPI) DispatchCommand(ctx context.Context, name, args string) (string, bool) {
	h.mu.RLock()
	var matched *ContributedCommand
	for key, cmd := range h.commands {
		_ = key
		if cmd.Name == name {
			c := cmd
			matched = &c
			break
		}
	}
	h.mu.RUnlock()
	if matched == nil {
		return "", false
	}
	result, err := matched.Handler(ctx, args)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), true
	}
	return result, true
}
