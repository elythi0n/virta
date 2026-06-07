package api

import (
	"context"
	"encoding/json"
	"net/http"
)

// ProfileExport is the portable serialisation of a workspace profile. Accounts appear as refs
// (platform + uid only, no tokens — tokens live in the OS keychain and are never exported), so an
// exported profile can be imported on another machine and re-authenticated independently.
type ProfileExport struct {
	SchemaVersion int             `json:"schema_version"` // bump on incompatible format changes
	Name          string          `json:"name"`
	Doc           json.RawMessage `json:"doc"` // the raw profile document (channels, filters, layouts, …)
	AccountRefs   []AccountRef    `json:"account_refs,omitempty"`
}

// AccountRef is the non-secret part of a connected account: enough to know "this profile expects a
// Twitch account for login X" without storing any credentials.
type AccountRef struct {
	Platform string `json:"platform"`
	Login    string `json:"login"`
}

// Portability is the profile import/export control surface. Implemented by the wiring layer and
// injected via SetPortability.
type Portability interface {
	// ExportProfile serialises a profile by id. Accounts are reduced to refs.
	ExportProfile(ctx context.Context, id string) (ProfileExport, error)
	// ImportProfile creates a new profile from the portable form; it never overwrites an existing
	// one (callers detect a name conflict and let the user rename).
	ImportProfile(ctx context.Context, p ProfileExport) (ProfileInfo, error)
}

// SetPortability installs the import/export controller.
func (s *Server) SetPortability(p Portability) { s.portability = p }

func (s *Server) handleExportProfile(w http.ResponseWriter, r *http.Request) {
	if s.portability == nil {
		http.Error(w, "portability unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing profile id", http.StatusBadRequest)
		return
	}
	exp, err := s.portability.ExportProfile(r.Context(), id)
	if err != nil {
		s.channelError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="virta-profile.json"`)
	writeJSON(w, exp)
}

func (s *Server) handleImportProfile(w http.ResponseWriter, r *http.Request) {
	if s.portability == nil {
		http.Error(w, "portability unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var exp ProfileExport
	if err := json.NewDecoder(r.Body).Decode(&exp); err != nil || exp.Name == "" {
		http.Error(w, "expected a valid profile export JSON", http.StatusBadRequest)
		return
	}
	if exp.SchemaVersion != 1 {
		http.Error(w, "unsupported schema_version (expected 1)", http.StatusBadRequest)
		return
	}
	info, err := s.portability.ImportProfile(r.Context(), exp)
	if err != nil {
		s.channelError(w, err)
		return
	}
	writeJSON(w, info)
}
