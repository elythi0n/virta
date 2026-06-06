package api

import (
	"net/http"
	"strings"

	"github.com/elythi0n/virta/internal/buildinfo"
)

// The API contract is generated from the one route table (server.routes()), so the published docs
// can never drift from what's actually wired. We emit OpenAPI 3.1 for the REST surface and a small
// AsyncAPI for the WebSocket event stream, and render a dependency-free /docs page from them.

func pathParams(path string) []map[string]any {
	var params []map[string]any
	for _, seg := range strings.Split(path, "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			name := seg[1 : len(seg)-1]
			params = append(params, map[string]any{
				"name": name, "in": "path", "required": true,
				"schema": map[string]any{"type": "string"},
			})
		}
	}
	return params
}

func operationID(method, path string) string {
	id := strings.ToLower(method) + strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(path, "/v1/", "/"), "{", ""), "}", "")
	id = strings.ReplaceAll(id, "/", "_")
	return strings.Trim(id, "_")
}

func (s *Server) openAPISpec() map[string]any {
	paths := map[string]any{}
	for _, rt := range s.routes() {
		item, ok := paths[rt.path].(map[string]any)
		if !ok {
			item = map[string]any{}
			paths[rt.path] = item
		}
		op := map[string]any{
			"summary":     rt.summary,
			"operationId": operationID(rt.method, rt.path),
			"tags":        []string{string(rt.scope)},
			"security":    []any{map[string]any{"bearerAuth": []string{}}},
			"x-required-scope": string(rt.scope),
			"responses": map[string]any{
				"200": map[string]any{"description": "OK", "content": map[string]any{"application/json": map[string]any{}}},
				"401": map[string]any{"description": "Missing or invalid token"},
				"403": map[string]any{"description": "Token lacks the required scope"},
			},
		}
		if params := pathParams(rt.path); len(params) > 0 {
			op["parameters"] = params
		}
		item[strings.ToLower(rt.method)] = op
	}
	tags := make([]map[string]any, 0, len(AllScopes))
	scopeDesc := map[Scope]string{
		ScopeRead:     "Read the feed, history, stats, capabilities, and profiles.",
		ScopeSend:     "Send messages through connected accounts.",
		ScopeModerate: "Moderation actions where the account is permitted.",
		ScopeControl:  "Switch profiles, join/leave channels, read settings.",
		ScopeAdmin:    "Settings write, token management, sign-in flows.",
	}
	for _, sc := range AllScopes {
		tags = append(tags, map[string]any{"name": string(sc), "description": scopeDesc[sc]})
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Virta local API",
			"version":     "1",
			"description": "The same `/v1` API the Virta frontends use. Authenticate with a bearer token (the root token from the loopback discovery file, or a scoped token minted in Settings → Integrations). Every endpoint is tagged with the scope a non-root token needs. The WebSocket event protocol is described in /v1/asyncapi.json.",
			"x-build":     buildinfo.String(),
		},
		"servers": []any{map[string]any{"url": "/", "description": "This daemon"}},
		"tags":    tags,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type": "http", "scheme": "bearer",
					"description": "Bearer token in the Authorization header (or a `token` query param for the WebSocket).",
				},
			},
		},
		"security": []any{map[string]any{"bearerAuth": []string{}}},
		"paths":    paths,
	}
}

// asyncAPISpec describes the WebSocket event stream: one channel (/v1/stream) carrying the sealed
// set of wire event types. Generated from the known event catalog so it stays in lockstep.
func asyncAPISpec() map[string]any {
	events := []struct{ name, desc string }{
		{"message", "A normalized chat message."},
		{"message_deleted", "A message was removed (moderator delete, or the author's)."},
		{"channel_clear", "A timeout/ban (one user) or a full chat clear."},
		{"state", "An adapter- or channel-level health transition."},
		{"chat_settings", "A channel's chat-mode settings (slow/followers/emote/unique)."},
		{"stats", "A channel's rolling activity (msg/s, unique chatters, top emotes)."},
		{"profile_changed", "The active workspace profile switched."},
		{"held", "A message AutoMod is holding for review."},
		{"held_resolved", "A held message was approved or denied."},
		{"plugin", "Data published by a plugin DataSource on a namespaced stream."},
	}
	msgs := map[string]any{}
	oneOf := make([]any, 0, len(events))
	for _, e := range events {
		msgs[e.name] = map[string]any{
			"name":        e.name,
			"summary":     e.desc,
			"payload":     map[string]any{"type": "object", "properties": map[string]any{"type": map[string]any{"const": e.name}, "seq": map[string]any{"type": "integer"}}},
		}
		oneOf = append(oneOf, map[string]any{"$ref": "#/components/messages/" + e.name})
	}
	return map[string]any{
		"asyncapi": "2.6.0",
		"info":     map[string]any{"title": "Virta event stream", "version": "1", "description": "Subscribe over the /v1/stream WebSocket. After connecting, send {\"action\":\"subscribe\",\"channels\":[\"platform:slug\"],\"since\":<seq>}. The server replays buffered events past `since` (dedupe by the monotonic `seq`)."},
		"channels": map[string]any{
			"/v1/stream": map[string]any{
				"subscribe": map[string]any{"message": map[string]any{"oneOf": oneOf}},
			},
		},
		"components": map[string]any{"messages": msgs},
	}
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.openAPISpec())
}

func (s *Server) handleAsyncAPI(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, asyncAPISpec())
}

func (s *Server) handleDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(docsHTML))
}
