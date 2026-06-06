package api

import "net/http"

// EmoteInfo is one usable emote for composer autocomplete: its code and a ready-to-show image URL.
type EmoteInfo struct {
	Code string `json:"code"`
	URL  string `json:"url"`
}

func (s *Server) handleListEmotes(w http.ResponseWriter, _ *http.Request) {
	if s.channels == nil {
		http.Error(w, "channel control unavailable", http.StatusServiceUnavailable)
		return
	}
	list := s.channels.Emotes()
	if list == nil {
		list = []EmoteInfo{}
	}
	writeJSON(w, map[string]any{"emotes": list})
}
