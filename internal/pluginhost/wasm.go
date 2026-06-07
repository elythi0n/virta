package pluginhost

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	extism "github.com/extism/go-sdk"
)

// WASMHookKind identifies which lifecycle hook to invoke.
type WASMHookKind string

const (
	HookOnMessage WASMHookKind = "on_message" // called for each incoming chat message
	HookOnEvent   WASMHookKind = "on_event"   // called for every pipeline event
	HookCommand   WASMHookKind = "command"    // called when a contributed slash-command fires
)

// HookInput is the JSON-serialisable payload sent to a WASM hook function.
type HookInput struct {
	Kind   WASMHookKind `json:"kind"`
	Plugin string       `json:"plugin"` // plugin id
	Data   any          `json:"data"`   // event/message/command args
}

// HookOutput is the expected return from a WASM hook function.
type HookOutput struct {
	// Actions is a list of typed actions the plugin wants the host to perform.
	Actions []json.RawMessage `json:"actions,omitempty"`
	// Error is non-empty if the hook signalled a non-fatal error.
	Error string `json:"error,omitempty"`
}

// WASMRuntime hosts WASM plugins using Extism (wazero-backed, pure Go).
// Each plugin is a separate Extism Plugin instance with its own memory.
// The host functions exposed to the WASM module are the declared-scope subset of the HostAPI.
type WASMRuntime struct {
	manifest   *Manifest
	installDir string
	plugin     *extism.Plugin
}

// NewWASMRuntime loads and initialises the WASM module for the plugin.
func NewWASMRuntime(ctx context.Context, manifest *Manifest, installDir string) (*WASMRuntime, error) {
	if manifest.Main.WASM == "" {
		return nil, fmt.Errorf("plugin %q has no WASM entry point", manifest.ID)
	}

	wasmPath := filepath.Join(installDir, manifest.Main.WASM)
	wasmData, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("wasm: read %q: %w", wasmPath, err)
	}

	// Define host functions available to the WASM module.
	// Only functions corresponding to declared scopes are registered.
	hostFunctions := buildHostFunctions(manifest)

	manifest_extism := extism.Manifest{
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmData, Name: manifest.ID},
		},
	}
	config := extism.PluginConfig{
		EnableWasi: true,
	}
	plug, err := extism.NewPlugin(ctx, manifest_extism, config, hostFunctions)
	if err != nil {
		return nil, fmt.Errorf("wasm: init plugin %q: %w", manifest.ID, err)
	}

	return &WASMRuntime{manifest: manifest, installDir: installDir, plugin: plug}, nil
}

// CallHook invokes a hook function in the WASM module with the given input.
// Unknown functions or plugin errors are returned as non-fatal errors.
func (r *WASMRuntime) CallHook(kind WASMHookKind, data any) (*HookOutput, error) {
	input := HookInput{Kind: kind, Plugin: r.manifest.ID, Data: data}
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("wasm: marshal hook input: %w", err)
	}

	_, out, err := r.plugin.Call(string(kind), raw)
	if err != nil {
		// A plugin error is non-fatal — log it but don't take down the host.
		return &HookOutput{Error: err.Error()}, nil
	}

	var result HookOutput
	if err := json.Unmarshal(out, &result); err != nil {
		// Plugin may return empty/no output for simple hooks.
		return &HookOutput{}, nil
	}
	return &result, nil
}

// Close frees the WASM runtime resources.
func (r *WASMRuntime) Close(ctx context.Context) {
	if r.plugin != nil {
		_ = r.plugin.Close(ctx)
	}
}

// buildHostFunctions returns the Extism host functions the plugin is allowed to call,
// filtered to its declared scopes.
func buildHostFunctions(m *Manifest) []extism.HostFunction {
	var fns []extism.HostFunction

	// Every plugin can log.
	fns = append(fns, extism.NewHostFunctionWithStack(
		"log",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			msg, _ := p.ReadString(stack[0])
			_ = msg // integrate with structured logging in wire-up
		},
		[]extism.ValueType{extism.ValueTypeI64},
		[]extism.ValueType{},
	))

	// ScopeRead — read-only queries will be dispatched via the host API at runtime.
	// The WASM module calls "virta_query" with a JSON request; the Go host dispatches it.
	if m.HasScope(ScopeRead) {
		fns = append(fns, extism.NewHostFunctionWithStack(
			"virta_query",
			func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
				// Placeholder: real implementation dispatches to the HostAPI.
				ptr, _ := p.WriteBytes([]byte(`{"ok":true}`))
				stack[0] = ptr
			},
			[]extism.ValueType{extism.ValueTypeI64, extism.ValueTypeI64},
			[]extism.ValueType{},
		))
	}

	return fns
}
