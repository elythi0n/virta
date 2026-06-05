package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

// Memory is a complete in-memory Store. It is the unit-test backend for the whole codebase
// and the reference implementation the storetest conformance suite runs against alongside
// the real SQLite/Postgres backends — so it stays honest (ADR-024). Safe for concurrent use.
//
// Timestamps come from an injected clock (ADR-024 determinism); ids are a deterministic
// monotonic counter (the real ULID generator arrives in step 0.4 for the SQL backends).
type Memory struct {
	clk clock.Clock

	mu            sync.Mutex
	seq           int64
	settings      map[string]Setting
	profiles      map[string]Profile
	profileByName map[string]string // name → id
	accounts      map[string]Account
	channels      map[string]Channel
	channelBySlug map[string]string // "platform|slug" → id
	messages      []StoredMessage
	emoteSets     map[string]EmoteSet
	emoteFiles    map[string]EmoteFile
}

// NewMemory creates an empty in-memory store using clk for timestamps.
func NewMemory(clk clock.Clock) *Memory {
	return &Memory{
		clk:           clk,
		settings:      map[string]Setting{},
		profiles:      map[string]Profile{},
		profileByName: map[string]string{},
		accounts:      map[string]Account{},
		channels:      map[string]Channel{},
		channelBySlug: map[string]string{},
		emoteSets:     map[string]EmoteSet{},
		emoteFiles:    map[string]EmoteFile{},
	}
}

func (m *Memory) nextID(prefix string) string {
	m.seq++
	return fmt.Sprintf("%s_%020d", prefix, m.seq)
}

func cloneJSON(b json.RawMessage) json.RawMessage {
	if b == nil {
		return nil
	}
	return append(json.RawMessage(nil), b...)
}

// Store lifecycle.
func (m *Memory) Migrate(context.Context) error { return nil }
func (m *Memory) Ping(context.Context) error    { return nil }
func (m *Memory) Close() error                  { return nil }

func (m *Memory) Settings() SettingsRepo { return memSettings{m} }
func (m *Memory) Profiles() ProfileRepo  { return memProfiles{m} }
func (m *Memory) Accounts() AccountRepo  { return memAccounts{m} }
func (m *Memory) Channels() ChannelRepo  { return memChannels{m} }
func (m *Memory) Messages() MessageRepo  { return memMessages{m} }
func (m *Memory) Emotes() EmoteRepo      { return memEmotes{m} }

// ---- settings ----

type memSettings struct{ m *Memory }

func (r memSettings) Get(_ context.Context, scope string) (Setting, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	s, ok := r.m.settings[scope]
	if !ok {
		return Setting{}, ErrNotFound
	}
	s.Data = cloneJSON(s.Data)
	return s, nil
}

func (r memSettings) Put(_ context.Context, s Setting) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	s.Data = cloneJSON(s.Data)
	s.UpdatedAt = r.m.clk.Now()
	r.m.settings[s.Scope] = s
	return nil
}

func (r memSettings) All(_ context.Context) ([]Setting, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	out := make([]Setting, 0, len(r.m.settings))
	for _, s := range r.m.settings {
		s.Data = cloneJSON(s.Data)
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Scope < out[j].Scope })
	return out, nil
}

// ---- profiles ----

type memProfiles struct{ m *Memory }

func (r memProfiles) Create(_ context.Context, name string, doc json.RawMessage) (Profile, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, exists := r.m.profileByName[name]; exists {
		return Profile{}, ErrConflict
	}
	now := r.m.clk.Now()
	p := Profile{
		ID:        r.m.nextID("prof"),
		Name:      name,
		Doc:       cloneJSON(doc),
		IsDefault: len(r.m.profiles) == 0, // first profile becomes default
		CreatedAt: now,
		UpdatedAt: now,
	}
	r.m.profiles[p.ID] = p
	r.m.profileByName[name] = p.ID
	out := p
	out.Doc = cloneJSON(p.Doc)
	return out, nil
}

func (r memProfiles) Get(_ context.Context, id string) (Profile, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	p, ok := r.m.profiles[id]
	if !ok {
		return Profile{}, ErrNotFound
	}
	p.Doc = cloneJSON(p.Doc)
	return p, nil
}

func (r memProfiles) GetByName(_ context.Context, name string) (Profile, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	id, ok := r.m.profileByName[name]
	if !ok {
		return Profile{}, ErrNotFound
	}
	p := r.m.profiles[id]
	p.Doc = cloneJSON(p.Doc)
	return p, nil
}

func (r memProfiles) List(_ context.Context) ([]Profile, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	out := make([]Profile, 0, len(r.m.profiles))
	for _, p := range r.m.profiles {
		p.Doc = cloneJSON(p.Doc)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r memProfiles) Update(_ context.Context, id string, doc json.RawMessage) (Profile, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	p, ok := r.m.profiles[id]
	if !ok {
		return Profile{}, ErrNotFound
	}
	p.Doc = cloneJSON(doc)
	p.UpdatedAt = r.m.clk.Now()
	r.m.profiles[id] = p
	out := p
	out.Doc = cloneJSON(p.Doc)
	return out, nil
}

func (r memProfiles) Delete(_ context.Context, id string) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	p, ok := r.m.profiles[id]
	if !ok {
		return ErrNotFound
	}
	delete(r.m.profiles, id)
	delete(r.m.profileByName, p.Name)
	return nil
}

func (r memProfiles) SetDefault(_ context.Context, id string) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.profiles[id]; !ok {
		return ErrNotFound
	}
	for pid, p := range r.m.profiles {
		p.IsDefault = pid == id
		r.m.profiles[pid] = p
	}
	return nil
}

func (r memProfiles) Default(_ context.Context) (Profile, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	for _, p := range r.m.profiles {
		if p.IsDefault {
			p.Doc = cloneJSON(p.Doc)
			return p, nil
		}
	}
	return Profile{}, ErrNotFound
}

// ---- accounts ----

type memAccounts struct{ m *Memory }

func (r memAccounts) Upsert(_ context.Context, a Account) (Account, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	now := r.m.clk.Now()
	// find existing by (platform, platform_uid)
	for id, ex := range r.m.accounts {
		if ex.Platform == a.Platform && ex.PlatformUID == a.PlatformUID {
			a.ID = id
			a.CreatedAt = ex.CreatedAt
			a.UpdatedAt = now
			a.Scopes = append([]string(nil), a.Scopes...)
			r.m.accounts[id] = a
			return a, nil
		}
	}
	a.ID = r.m.nextID("acct")
	a.CreatedAt = now
	a.UpdatedAt = now
	a.Scopes = append([]string(nil), a.Scopes...)
	r.m.accounts[a.ID] = a
	return a, nil
}

func (r memAccounts) Get(_ context.Context, id string) (Account, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	a, ok := r.m.accounts[id]
	if !ok {
		return Account{}, ErrNotFound
	}
	a.Scopes = append([]string(nil), a.Scopes...)
	return a, nil
}

func (r memAccounts) List(_ context.Context) ([]Account, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	out := make([]Account, 0, len(r.m.accounts))
	for _, a := range r.m.accounts {
		a.Scopes = append([]string(nil), a.Scopes...)
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r memAccounts) ListByPlatform(_ context.Context, p platform.Platform) ([]Account, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	out := make([]Account, 0)
	for _, a := range r.m.accounts {
		if a.Platform == p {
			a.Scopes = append([]string(nil), a.Scopes...)
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r memAccounts) Delete(_ context.Context, id string) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.accounts[id]; !ok {
		return ErrNotFound
	}
	delete(r.m.accounts, id)
	return nil
}

// ---- channels ----

type memChannels struct{ m *Memory }

func channelKey(p platform.Platform, slug string) string { return string(p) + "|" + slug }

func (r memChannels) Upsert(_ context.Context, c Channel) (Channel, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	key := channelKey(c.Platform, c.Slug)
	if id, ok := r.m.channelBySlug[key]; ok {
		ex := r.m.channels[id]
		c.ID = ex.ID
		c.Meta = cloneJSON(c.Meta)
		r.m.channels[id] = c
		return c, nil
	}
	c.ID = r.m.nextID("chan")
	c.Meta = cloneJSON(c.Meta)
	r.m.channels[c.ID] = c
	r.m.channelBySlug[key] = c.ID
	return c, nil
}

func (r memChannels) GetBySlug(_ context.Context, p platform.Platform, slug string) (Channel, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	id, ok := r.m.channelBySlug[channelKey(p, slug)]
	if !ok {
		return Channel{}, ErrNotFound
	}
	c := r.m.channels[id]
	c.Meta = cloneJSON(c.Meta)
	return c, nil
}

func (r memChannels) List(_ context.Context) ([]Channel, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	out := make([]Channel, 0, len(r.m.channels))
	for _, c := range r.m.channels {
		c.Meta = cloneJSON(c.Meta)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r memChannels) Delete(_ context.Context, id string) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	c, ok := r.m.channels[id]
	if !ok {
		return ErrNotFound
	}
	delete(r.m.channels, id)
	delete(r.m.channelBySlug, channelKey(c.Platform, c.Slug))
	return nil
}

// ---- messages ----

type memMessages struct{ m *Memory }

func (r memMessages) Append(_ context.Context, msgs []platform.UnifiedMessage) error {
	// Choke point (ADR-014): refuse the whole batch if any message is ephemeral.
	for i := range msgs {
		if msgs[i].Ephemeral {
			return ErrEphemeral
		}
	}
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	for i := range msgs {
		um := &msgs[i]
		segs, err := json.Marshal(um.Segments)
		if err != nil {
			return fmt.Errorf("store: marshal segments: %w", err)
		}
		r.m.messages = append(r.m.messages, StoredMessage{
			ID:         um.ID,
			ChannelID:  um.Channel.ID,
			Platform:   um.Platform,
			Type:       um.Type,
			AuthorUID:  um.Author.ID,
			AuthorName: um.Author.DisplayName,
			Body:       um.PlainText(),
			Segments:   segs,
			SentAt:     um.SentAt,
			ReceivedAt: um.ReceivedAt,
		})
	}
	return nil
}

func (r memMessages) History(_ context.Context, q HistoryQuery) ([]StoredMessage, error) {
	limit := q.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	// newest-first by ULID id
	matched := make([]StoredMessage, 0)
	for _, sm := range r.m.messages {
		if sm.ChannelID != q.ChannelID {
			continue
		}
		if q.Before != "" && sm.ID >= q.Before {
			continue
		}
		sm.Segments = cloneJSON(sm.Segments)
		matched = append(matched, sm)
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID > matched[j].ID })
	if len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

func (r memMessages) MarkDeleted(_ context.Context, id string) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	for i := range r.m.messages {
		if r.m.messages[i].ID == id {
			r.m.messages[i].Deleted = true
			return nil
		}
	}
	return ErrNotFound
}

func (r memMessages) Sweep(_ context.Context, channelID string, olderThan time.Time) (int, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	kept := r.m.messages[:0]
	removed := 0
	for _, sm := range r.m.messages {
		if sm.ChannelID == channelID && sm.ReceivedAt.Before(olderThan) {
			removed++
			continue
		}
		kept = append(kept, sm)
	}
	r.m.messages = kept
	return removed, nil
}

// ---- emotes ----

type memEmotes struct{ m *Memory }

func (r memEmotes) PutSet(_ context.Context, s EmoteSet) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	s.Data = cloneJSON(s.Data)
	r.m.emoteSets[s.Key] = s
	return nil
}

func (r memEmotes) GetSet(_ context.Context, key string) (EmoteSet, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	s, ok := r.m.emoteSets[key]
	if !ok {
		return EmoteSet{}, ErrNotFound
	}
	s.Data = cloneJSON(s.Data)
	return s, nil
}

func (r memEmotes) PutFile(_ context.Context, f EmoteFile) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	r.m.emoteFiles[f.URLHash] = f
	return nil
}

func (r memEmotes) GetFile(_ context.Context, urlHash string) (EmoteFile, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	f, ok := r.m.emoteFiles[urlHash]
	if !ok {
		return EmoteFile{}, ErrNotFound
	}
	return f, nil
}

var _ Store = (*Memory)(nil)
