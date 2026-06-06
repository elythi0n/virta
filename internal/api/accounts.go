package api

import (
	"context"
	"net/http"
)

// AccountInfo is one connected account, as served by GET /v1/accounts — the identity and scopes,
// never the token (which stays in the keychain).
type AccountInfo struct {
	ID          string   `json:"id"`
	Platform    string   `json:"platform"`
	Login       string   `json:"login"`
	DisplayName string   `json:"display_name,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

// Accounts is the connected-accounts control, implemented by the wiring layer and injected via
// SetAccounts. Disconnect revokes an account: its keychain secret and row are removed and the
// platform reverts to anonymous read-only.
type Accounts interface {
	Accounts() []AccountInfo
	Disconnect(ctx context.Context, id string) error
}

// SetAccounts installs the accounts controller.
func (s *Server) SetAccounts(a Accounts) { s.accounts = a }

func (s *Server) handleListAccounts(w http.ResponseWriter, _ *http.Request) {
	if s.accounts == nil {
		http.Error(w, "accounts unavailable", http.StatusServiceUnavailable)
		return
	}
	list := s.accounts.Accounts()
	if list == nil {
		list = []AccountInfo{}
	}
	writeJSON(w, map[string]any{"accounts": list})
}

func (s *Server) handleDisconnectAccount(w http.ResponseWriter, r *http.Request) {
	if s.accounts == nil {
		http.Error(w, "accounts unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "expected an account id", http.StatusBadRequest)
		return
	}
	if err := s.accounts.Disconnect(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
