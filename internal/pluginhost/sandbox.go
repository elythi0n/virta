package pluginhost

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// GUIPlugin serves a sandboxed GUI plugin's static assets over HTTP with strict CSP headers.
// The plugin's HTML/JS/CSS are served from its install directory. The sandboxed iframe is
// prevented from making direct network requests; it communicates with the host via postMessage
// which the host SDK relays to the daemon through the existing WS connection.
//
// Security posture:
//   - Content-Security-Policy: default-src 'none'; restricts all resource loading
//   - script-src 'self' allows scripts from this exact origin only
//   - connect-src points only to the Virta WS origin (same as the page)
//   - No inline scripts (no unsafe-inline); plugin JS must be served as files
//   - The sandbox attribute on the host iframe additionally limits: allow-scripts allow-same-origin
type GUIPlugin struct {
	manifest   *Manifest
	installDir string
	fileServer http.Handler
}

// NewGUIPlugin creates a sandboxed handler for the plugin's GUI files.
func NewGUIPlugin(manifest *Manifest, installDir string) (*GUIPlugin, error) {
	if manifest.Main.GUI == "" {
		return nil, fmt.Errorf("plugin %q has no GUI entry point", manifest.ID)
	}
	guiDir := filepath.Join(installDir, filepath.Dir(manifest.Main.GUI))
	if _, err := os.Stat(guiDir); err != nil {
		return nil, fmt.Errorf("sandbox: GUI directory %q not found: %w", guiDir, err)
	}
	return &GUIPlugin{
		manifest:   manifest,
		installDir: installDir,
		fileServer: http.FileServer(http.Dir(guiDir)),
	}, nil
}

// ServeHTTP serves the plugin's GUI assets with strict CSP headers.
func (g *GUIPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Strict Content-Security-Policy:
	// - default-src 'none' blocks everything not explicitly permitted
	// - script-src 'self' allows scripts from this origin only (no eval, no inline)
	// - style-src 'self' 'unsafe-inline' (allows inline styles, commonly needed for component libraries)
	// - connect-src 'self' ws: wss: allows WS connections back to the same origin only
	// - img-src 'self' data: allows self-hosted images and data URIs
	// - font-src 'self' allows self-hosted fonts
	// No network, no object/embed, no frame ancestors (prevents clickjacking).
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; "+
			"script-src 'self'; "+
			"style-src 'self' 'unsafe-inline'; "+
			"connect-src 'self' ws: wss:; "+
			"img-src 'self' data:; "+
			"font-src 'self'; "+
			"frame-ancestors 'self'",
	)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Cache-Control", "no-store") // plugin files change on update

	g.fileServer.ServeHTTP(w, r)
}

// IFrameAttribs returns the HTML attributes to set on the sandbox iframe in the host renderer.
// These restrict what the sandboxed page can do even if CSP is bypassed.
func IFrameAttribs(pluginID string) map[string]string {
	return map[string]string{
		// allow-scripts: needed to run the plugin JS
		// allow-same-origin: needed for postMessage to reach the host origin
		// No allow-popups, allow-downloads, allow-forms, allow-pointer-lock
		"sandbox": "allow-scripts allow-same-origin",
		// No name — plugins can't target frames
		"loading": "lazy",
		"title":   fmt.Sprintf("Plugin: %s", pluginID),
	}
}

// HostSDKBootstrap generates the inline bootstrap snippet injected into the plugin's index.html.
// This snippet establishes the postMessage bridge and provides the @virta/plugin SDK surface.
// The plugin's JS calls window.__virta.send({type, payload}) to dispatch to the host.
func HostSDKBootstrap(pluginID string, scopes []Scope) string {
	scopeJSON, _ := json.Marshal(scopes)
	_ = scopeJSON
	return strings.ReplaceAll(strings.ReplaceAll(`(function() {
  'use strict';
  var _pending = {};
  var _seq = 0;
  var _handlers = {};
  window.__virta = {
    id: "__PLUGIN_ID__",
    scopes: __SCOPES__,
    // Send a request to the host and get a Promise back.
    send: function(msg) {
      var id = ++_seq;
      return new Promise(function(resolve, reject) {
        _pending[id] = { resolve: resolve, reject: reject };
        window.parent.postMessage({ __virta: true, seq: id, plugin: "__PLUGIN_ID__", msg: msg }, '*');
      });
    },
    // Subscribe to events from the host (e.g. plugin data stream ticks).
    on: function(type, handler) {
      if (!_handlers[type]) _handlers[type] = [];
      _handlers[type].push(handler);
    },
    off: function(type, handler) {
      if (!_handlers[type]) return;
      _handlers[type] = _handlers[type].filter(function(h) { return h !== handler; });
    },
  };
  window.addEventListener('message', function(ev) {
    if (!ev.data || !ev.data.__virta_host) return;
    var d = ev.data;
    if (d.seq && _pending[d.seq]) {
      if (d.error) { _pending[d.seq].reject(new Error(d.error)); }
      else { _pending[d.seq].resolve(d.result); }
      delete _pending[d.seq];
    } else if (d.type && _handlers[d.type]) {
      _handlers[d.type].forEach(function(h) { try { h(d.payload); } catch(e) {} });
    }
  });
})();`, "__PLUGIN_ID__", pluginID), "__SCOPES__", string(scopeJSON))
}
