package intel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// MCPServer implements a minimal Model Context Protocol server exposing the tool belt over
// stdio (for Claude Desktop) and HTTP (for web MCP clients). JSON-RPC 2.0 throughout.
// Security: the HTTP transport requires the daemon bearer token. Tools are read-only.

// MCPHandler returns an http.Handler for the /mcp endpoint (HTTP transport).
func (tb *ToolBelt) MCPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSONMCP(w, tb.manifest())
			return
		}
		var req jsonRPCReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeMCPError(w, nil, -32700, "parse error")
			return
		}
		resp := tb.handleRPC(r.Context(), req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
}

// ServeStdio runs the MCP server on stdin/stdout, blocking until EOF.
// This is the transport Claude Desktop uses when `virtad mcp` is configured as an MCP server.
func (tb *ToolBelt) ServeStdio(r io.Reader, w io.Writer) {
	dec := json.NewDecoder(r)
	enc := json.NewEncoder(w)
	ctx := context.Background()
	for {
		var req jsonRPCReq
		if err := dec.Decode(&req); err != nil {
			return
		}
		_ = enc.Encode(tb.handleRPC(ctx, req))
	}
}

// ---- JSON-RPC 2.0 types ----

type jsonRPCReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResp struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *mcpError `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpManifest struct {
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Description string       `json:"description"`
	Tools       []ToolSchema `json:"tools"`
}

func (tb *ToolBelt) manifest() mcpManifest {
	return mcpManifest{
		Name:        "virta",
		Version:     "1.0",
		Description: "Read-only tools over Virta's logged chat history: search, top chatters, stats, summarisation.",
		Tools:       Descriptions(),
	}
}

func (tb *ToolBelt) handleRPC(ctx context.Context, req jsonRPCReq) jsonRPCResp {
	id := req.ID
	switch req.Method {
	case "initialize":
		return jsonRPCResp{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "virta", "version": "1.0"},
		}}
	case "tools/list", "list_tools":
		return jsonRPCResp{JSONRPC: "2.0", ID: id, Result: map[string]any{"tools": Descriptions()}}
	case "tools/call", "call_tool":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return errResp(id, -32600, "invalid params: "+err.Error())
		}
		result, err := tb.Dispatch(ctx, p.Name, p.Arguments)
		if err != nil {
			return errResp(id, -32000, err.Error())
		}
		return jsonRPCResp{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []map[string]any{{"type": "text", "text": mustMCPJSON(result)}},
		}}
	default:
		return errResp(id, -32601, "method not found: "+req.Method)
	}
}

func errResp(id any, code int, msg string) jsonRPCResp {
	return jsonRPCResp{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: code, Message: msg}}
}

func mustMCPJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(b)
}

func writeJSONMCP(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeMCPError(w http.ResponseWriter, id any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(errResp(id, code, msg))
}
