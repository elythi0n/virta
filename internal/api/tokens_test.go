package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestTokens_MintListRevoke(t *testing.T) {
	s := start(t)
	store := NewMemTokens()
	s.SetTokens(store)

	// Mint a token.
	body := `{"name":"stream deck","scopes":["read","send"]}`
	code, resp := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/tokens?token="+s.Token(), []byte(body))
	if code != http.StatusOK {
		t.Fatalf("mint status = %d (%s)", code, resp)
	}
	var minted MintedToken
	if err := json.Unmarshal(resp, &minted); err != nil {
		t.Fatalf("unmarshal minted: %v", err)
	}
	if minted.Token == "" || minted.ID == "" {
		t.Fatalf("missing token or id: %+v", minted)
	}
	if minted.Name != "stream deck" {
		t.Errorf("name = %q", minted.Name)
	}

	// List: the minted token is visible (without its secret).
	code, resp = authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/tokens?token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("list status = %d", code)
	}
	var list struct {
		Tokens []TokenInfo `json:"tokens"`
	}
	if err := json.Unmarshal(resp, &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list.Tokens) != 1 || list.Tokens[0].ID != minted.ID {
		t.Errorf("list = %+v", list.Tokens)
	}

	// Revoke.
	code, _ = authedReq(t, http.MethodDelete, "http://"+s.Addr()+"/v1/tokens/"+minted.ID+"?token="+s.Token(), nil)
	if code != http.StatusNoContent {
		t.Errorf("revoke status = %d, want 204", code)
	}
	// After revoke the list is empty.
	_, resp = authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/tokens?token="+s.Token(), nil)
	if err := json.Unmarshal(resp, &list); err == nil && len(list.Tokens) != 0 {
		t.Error("token still listed after revoke")
	}
}

func TestTokens_ScopedTokenCanRead(t *testing.T) {
	s := start(t)
	store := NewMemTokens()
	s.SetTokens(store)

	minted, _ := store.Mint("bot", []Scope{ScopeRead})
	code, _ := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/channels?token="+minted.Token, nil)
	if code != http.StatusOK && code != http.StatusServiceUnavailable {
		t.Errorf("read-scoped token status = %d, want 200 or 503 (no channels)", code)
	}
}

func TestTokens_ScopedTokenMissingScope(t *testing.T) {
	s := start(t)
	store := NewMemTokens()
	s.SetTokens(store)

	minted, _ := store.Mint("read-only", []Scope{ScopeRead})
	code, resp := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/send?token="+minted.Token, []byte(`{"channels":["twitch:x"],"text":"hi"}`))
	if code != http.StatusForbidden {
		t.Errorf("send with read-only token status = %d, want 403; body=%s", code, resp)
	}
}

func TestTokens_BadScopeRejected(t *testing.T) {
	s := start(t)
	s.SetTokens(NewMemTokens())
	code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/tokens?token="+s.Token(), []byte(`{"name":"x","scopes":["superpowers"]}`))
	if code != http.StatusBadRequest {
		t.Errorf("bad scope status = %d, want 400", code)
	}
}

func TestTokens_UnavailableUntilSet(t *testing.T) {
	s := start(t)
	code, _ := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/tokens?token="+s.Token(), nil)
	if code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 before SetTokens", code)
	}
}

func TestTokens_HashDeterministic(t *testing.T) {
	h1 := HashToken("vk_abc")
	h2 := HashToken("vk_abc")
	if h1 != h2 {
		t.Error("HashToken is not deterministic")
	}
	if strings.ContainsAny(h1, " \n\t") || len(h1) < 16 {
		t.Errorf("unexpected hash shape: %q", h1)
	}
}
